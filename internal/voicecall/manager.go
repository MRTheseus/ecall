package voicecall

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iniwex5/vohive/internal/device"
	"github.com/iniwex5/vohive/internal/modem"
	"github.com/iniwex5/vohive/pkg/logger"
)

func executeWorkerAT(w *device.Worker, cmd string, timeout time.Duration) (string, error) {
	if w == nil {
		return "", fmt.Errorf("设备未找到")
	}
	port := w.ResolvedATPort()
	if port == "" {
		port = w.Config.ATPort
	}
	if port == "" {
		port = "/dev/ttyUSB3"
	}
	session, err := modem.NewSerialAT(port, 115200, 8, 1, "N")
	if err != nil {
		return "", fmt.Errorf("打开 AT 串口 %s 失败: %w", port, err)
	}
	defer session.Close()
	return session.Execute(cmd, timeout)
}

// Manager 管理整个系统的蜂窝语音呼叫会话与 WebRTC 网关
type Manager struct {
	mu           sync.RWMutex
	pool         *device.Pool
	currentCall  *CallSession
	audioBridge  *AudioBridge
	webrtcGW     *WebRTCGateway
	subscribers  map[chan CallEvent]struct{}
	subscribersMu sync.RWMutex
}

func NewManager(pool *device.Pool) *Manager {
	bridge := NewAudioBridge("")
	gw := NewWebRTCGateway(bridge)

	m := &Manager{
		pool:        pool,
		audioBridge: bridge,
		webrtcGW:    gw,
		subscribers: make(map[chan CallEvent]struct{}),
	}

	gw.onDisconnect = func() {
		logger.Info("WebRTC 连接断开，检查呼叫状态")
	}

	return m
}

// GetStatus 获取当前通话状态
func (m *Manager) GetStatus() *CallSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentCall == nil {
		return &CallSession{
			State: CallStateIdle,
		}
	}
	// 计算实时时长
	copySession := *m.currentCall
	if copySession.ConnectedAt != nil && copySession.State == CallStateActive {
		copySession.DurationSec = int64(time.Since(*copySession.ConnectedAt).Seconds())
	}
	return &copySession
}

// Dial 发起呼叫
func (m *Manager) Dial(ctx context.Context, req DialRequest) (*CallSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req.Number = strings.TrimSpace(req.Number)
	if req.Number == "" {
		return nil, fmt.Errorf("号码不能为空")
	}

	// 获取指定或默认设备
	var worker *device.Worker
	if req.DeviceID != "" {
		worker = m.pool.GetWorker(req.DeviceID)
	} else {
		// 查找首个可用 Worker
		for _, w := range m.pool.GetAllWorkers() {
			worker = w
			break
		}
	}

	if worker == nil {
		return nil, fmt.Errorf("无可用模组设备")
	}

	// 如果当前已有进行中的通话，先挂断
	if m.currentCall != nil && m.currentCall.State != CallStateIdle && m.currentCall.State != CallStateTerminated {
		_ = m.hangupLocked("new_dial")
	}

	now := time.Now()
	session := &CallSession{
		ID:           uuid.New().String(),
		DeviceID:     worker.ID,
		RemoteNumber: req.Number,
		Direction:    DirectionOutbound,
		State:        CallStateDialing,
		StartedAt:    &now,
	}
	m.currentCall = session

	// 异步向模组发送 AT 拨号指令
	go func(dev *device.Worker, target string, sessID string) {
		dialCmd := fmt.Sprintf("ATD%s;", target)
		logger.Info("执行 4G VoLTE 拨号", "device", dev.ID, "number", target, "cmd", dialCmd)

		resp, err := executeWorkerAT(dev, dialCmd, 10*time.Second)
		if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
			logger.Warn("4G 拨号失败", "device", dev.ID, "err", err, "resp", resp)
			m.mu.Lock()
			if m.currentCall != nil && m.currentCall.ID == sessID {
				m.currentCall.State = CallStateTerminated
				m.currentCall.HangupReason = "dial_failed"
				m.broadcastEventLocked("state_change", m.currentCall, "拨号失败: "+resp)
			}
			m.mu.Unlock()
			return
		}

		// 标记为正在振铃/接通
		m.mu.Lock()
		if m.currentCall != nil && m.currentCall.ID == sessID {
			connTime := time.Now()
			m.currentCall.State = CallStateActive
			m.currentCall.ConnectedAt = &connTime
			_ = m.audioBridge.Open(context.Background())
			m.broadcastEventLocked("state_change", m.currentCall, "已呼出")
		}
		m.mu.Unlock()
	}(worker, req.Number, session.ID)

	m.broadcastEventLocked("state_change", session, "正在呼出...")
	return session, nil
}

// Answer 接听来电
func (m *Manager) Answer(ctx context.Context) (*CallSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentCall == nil || m.currentCall.State != CallStateRinging {
		return nil, fmt.Errorf("当前没有来电振铃")
	}

	worker := m.pool.GetWorker(m.currentCall.DeviceID)
	if worker == nil {
		return nil, fmt.Errorf("设备未找到")
	}

	// 发送 ATA 接听指令
	resp, err := executeWorkerAT(worker, "ATA", 10*time.Second)
	if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
		return nil, fmt.Errorf("接听指令失败: %w, resp: %s", err, resp)
	}

	now := time.Now()
	m.currentCall.State = CallStateActive
	m.currentCall.ConnectedAt = &now
	_ = m.audioBridge.Open(context.Background())

	m.broadcastEventLocked("state_change", m.currentCall, "已接通")
	return m.currentCall, nil
}

// Hangup 挂断通话
func (m *Manager) Hangup(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.hangupLocked("user_hangup")
}

func (m *Manager) hangupLocked(reason string) error {
	if m.currentCall == nil {
		return nil
	}

	worker := m.pool.GetWorker(m.currentCall.DeviceID)
	if worker != nil {
		_, _ = executeWorkerAT(worker, "ATH", 5*time.Second) // 挂断
	}

	now := time.Now()
	m.currentCall.State = CallStateTerminated
	m.currentCall.EndedAt = &now
	m.currentCall.HangupReason = reason
	if m.currentCall.ConnectedAt != nil {
		m.currentCall.DurationSec = int64(now.Sub(*m.currentCall.ConnectedAt).Seconds())
	}

	_ = m.audioBridge.Close()
	m.webrtcGW.Close()

	m.broadcastEventLocked("hangup", m.currentCall, "通话已结束")
	return nil
}

// SendDTMF 发送按键音
func (m *Manager) SendDTMF(ctx context.Context, digit string) error {
	m.mu.RLock()
	call := m.currentCall
	m.mu.RUnlock()

	if call == nil || call.State != CallStateActive {
		return fmt.Errorf("当前不在通话中")
	}

	worker := m.pool.GetWorker(call.DeviceID)
	if worker == nil {
		return fmt.Errorf("设备未找到")
	}

	dtmfCmd := fmt.Sprintf("AT+VTS=\"%s\"", digit)
	_, err := executeWorkerAT(worker, dtmfCmd, 5*time.Second)
	return err
}

// HandleWebRTCOffer 处理 WebRTC Offer
func (m *Manager) HandleWebRTCOffer(ctx context.Context, offerSDP string) (string, error) {
	return m.webrtcGW.HandleOffer(ctx, offerSDP)
}

// HandleICECandidate 处理 ICE Candidate
func (m *Manager) HandleICECandidate(candidate WebRTCCandidateRequest) error {
	return m.webrtcGW.AddICECandidate(webrtcCandidateInit(candidate))
}

// SubscribeEvents 订阅通话状态变更事件通道
func (m *Manager) SubscribeEvents() (chan CallEvent, func()) {
	ch := make(chan CallEvent, 16)
	m.subscribersMu.Lock()
	m.subscribers[ch] = struct{}{}
	m.subscribersMu.Unlock()

	// 立即推送一次当前状态
	status := m.GetStatus()
	ch <- CallEvent{
		Type:      "status",
		Session:   status,
		Timestamp: time.Now().UnixMilli(),
	}

	unsubscribe := func() {
		m.subscribersMu.Lock()
		delete(m.subscribers, ch)
		m.subscribersMu.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

func (m *Manager) broadcastEventLocked(eventType string, session *CallSession, msg string) {
	m.subscribersMu.RLock()
	defer m.subscribersMu.RUnlock()

	ev := CallEvent{
		Type:      eventType,
		Session:   session,
		Timestamp: time.Now().UnixMilli(),
		Message:   msg,
	}

	for ch := range m.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

// OnIncomingCall 模组收到来电 URC（RING / +CLIP）时触发
func (m *Manager) OnIncomingCall(deviceID, remoteNumber string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	session := &CallSession{
		ID:           uuid.New().String(),
		DeviceID:     deviceID,
		RemoteNumber: remoteNumber,
		Direction:    DirectionInbound,
		State:        CallStateRinging,
		StartedAt:    &now,
	}
	m.currentCall = session
	m.broadcastEventLocked("incoming", session, "收到来电: "+remoteNumber)
}
