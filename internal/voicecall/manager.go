package voicecall

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iniwex5/vohive/internal/db"
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
	serialCtrl   *SerialController
	trackerMu    sync.Mutex
	stopTracker  chan struct{}
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
	m.serialCtrl = NewSerialController("/dev/ttyUSB2", 115200)
	m.serialCtrl.OnHangup = func(reason string) {
		m.handleSerialHangup(reason)
	}
	m.serialCtrl.OnIncoming = func(remoteNumber string) {
		m.OnIncomingCall("dji4g", remoteNumber)
	}
	go m.serialCtrl.Start(context.Background())

	gw.onDisconnect = func() {
		logger.Info("WebRTC disconnected")
	}

	// 服务冷启动时强制清场残留并重置模组语音底座
	ResetQDC507VoiceRoute()

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
	req.Number = strings.TrimSpace(req.Number)
	if req.Number == "" {
		return nil, fmt.Errorf("号码不能为空")
	}

	// 先在锁外清理基带残留状态（AT+CHUP 会触发 NO CARRIER URC，必须在拿锁前完成）
	_, _ = m.serialCtrl.Execute("AT+CHUP", 2*time.Second)
	time.Sleep(300 * time.Millisecond)

	m.mu.Lock()
	defer m.mu.Unlock()

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

	_ = m.audioBridge.Open(context.Background())
	EnsureQDC507VoiceRoute()

	go func(target string, sessID string) {
		dialCmd := fmt.Sprintf("ATD%s;", target)
		logger.Info("Dialing", "number", target, "cmd", dialCmd)

		resp, err := m.serialCtrl.Execute(dialCmd, 10*time.Second)
		if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
			logger.Warn("Dial failed or rejected immediately", "resp", resp, "err", err)
			m.mu.Lock()
			if m.currentCall != nil && m.currentCall.ID == sessID {
				m.currentCall.State = CallStateTerminated
				m.currentCall.HangupReason = "dial_failed"
				m.broadcastEventLocked("hangup", m.currentCall, "对方拒接或拨号失败")
			}
			m.mu.Unlock()
			return
		}
		logger.Info("ATD OK, starting call tracker", "sessID", sessID)
		m.startCallTracker(sessID)
	}(req.Number, session.ID)

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
	resp, err := m.serialCtrl.Execute("ATA", 4*time.Second)
	if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
		return nil, fmt.Errorf("接听指令失败: %w, resp: %s", err, resp)
	}

	now := time.Now()
	m.currentCall.State = CallStateActive
	m.currentCall.ConnectedAt = &now
	_ = m.audioBridge.Open(context.Background())
	EnsureQDC507VoiceRoute()

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
	if m.currentCall == nil || m.currentCall.State == CallStateTerminated {
		return nil // 幂等保护：已结束则直接返回
	}

	// 如果不是由串口URC触发的挂断，才发送 ATH 指令
	if reason != "no_carrier" && reason != "busy" && reason != "no_answer" && reason != "remote_canceled" {
		_, _ = m.serialCtrl.Execute("ATH", 4*time.Second)
	}

	now := time.Now()
	m.currentCall.State = CallStateTerminated
	m.currentCall.EndedAt = &now
	m.currentCall.HangupReason = reason
	if m.currentCall.ConnectedAt != nil {
		m.currentCall.DurationSec = int64(now.Sub(*m.currentCall.ConnectedAt).Seconds())
	}

	// 异步持久化通话历史记录
	go func(call CallSession, endTime time.Time) {
		startTime := time.Now()
		if call.StartedAt != nil {
			startTime = *call.StartedAt
		}
		record := db.CallRecord{
			SessionID:    call.ID,
			DeviceID:     call.DeviceID,
			RemoteNumber: call.RemoteNumber,
			Direction:    string(call.Direction),
			DurationSec:  int(call.DurationSec),
			HangupReason: call.HangupReason,
			StartedAt:    startTime,
			ConnectedAt:  call.ConnectedAt,
			EndedAt:      endTime,
		}
		if call.ConnectedAt != nil {
			record.State = "completed"
		} else if call.Direction == DirectionInbound {
			record.State = "missed"
		} else if call.HangupReason == "busy" {
			record.State = "busy"
		} else {
			record.State = "canceled"
		}
		if err := db.SaveCallRecord(&record); err != nil {
			logger.Warn("保存通话记录失败", "err", err)
		}
	}(*m.currentCall, now)

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
	_, err := m.serialCtrl.Execute(dtmfCmd, 4*time.Second)
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
func (m *Manager) handleSerialHangup(reason string) {
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = m.hangupLocked(reason)
	}()
}

func (m *Manager) startCallTracker(sessID string) {
	m.trackerMu.Lock()
	if m.stopTracker != nil {
		close(m.stopTracker)
	}
	stopCh := make(chan struct{})
	m.stopTracker = stopCh
	m.trackerMu.Unlock()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		emptyCount := 0

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				m.mu.RLock()
				cur := m.currentCall
				m.mu.RUnlock()

				if cur == nil || cur.ID != sessID || cur.State == CallStateTerminated {
					return
				}

				logger.Info("Tracker: polling CLCC", "sessID", sessID[:8])
				resp, err := m.serialCtrl.Execute("AT+CLCC", 2*time.Second)
				if err != nil {
					logger.Warn("Tracker: CLCC error", "err", err)
					continue
				}
				logger.Info("Tracker: CLCC response", "resp", resp)

				if strings.Contains(resp, "NO CARRIER") || strings.Contains(resp, "BUSY") {
					m.mu.Lock()
					if m.currentCall != nil && m.currentCall.ID == sessID {
						_ = m.hangupLocked("busy")
					}
					m.mu.Unlock()
					return
				}

				entries := parseCLCC(resp)

				// 寻找与当前会话匹配的呼叫条目
				var matchedEntry *CLCCEntry
				cleanRemote := cleanPhoneNumber(cur.RemoteNumber)
				for i := range entries {
					e := &entries[i]
					cleanNum := cleanPhoneNumber(e.Number)
					if cleanNum != "" && cleanRemote != "" {
						if strings.HasSuffix(cleanNum, cleanRemote) || strings.HasSuffix(cleanRemote, cleanNum) {
							matchedEntry = e
							break
						}
					}
				}
				// 若未通过号码匹配（部分基带早期不返回号码），按方向匹配
				if matchedEntry == nil && len(entries) > 0 {
					expectedDir := 0 // MO
					if cur.Direction == DirectionInbound {
						expectedDir = 1 // MT
					}
					for i := range entries {
						if entries[i].Dir == expectedDir {
							matchedEntry = &entries[i]
							break
						}
					}
				}

				if matchedEntry == nil {
					// 我们的通话已不在活跃列表中！
					emptyCount++
					if emptyCount >= 2 {
						m.mu.Lock()
						if m.currentCall != nil && m.currentCall.ID == sessID {
							reason := "call_ended"
							if m.currentCall.Direction == DirectionInbound && m.currentCall.State == CallStateRinging {
								reason = "remote_canceled"
							} else if m.currentCall.Direction == DirectionOutbound && (m.currentCall.State == CallStateDialing || m.currentCall.State == CallStateRinging) {
								reason = "busy"
							}
							_ = m.hangupLocked(reason)
						}
						m.mu.Unlock()
						return
					}
					continue
				}

				emptyCount = 0
				entry := *matchedEntry

				m.mu.Lock()
				if m.currentCall != nil && m.currentCall.ID == sessID {
					switch entry.Stat {
					case 0:
						if m.currentCall.State != CallStateActive {
							m.currentCall.State = CallStateActive
							m.broadcastEventLocked("connected", m.currentCall, "")
						}
					case 3:
						if m.currentCall.State != CallStateRinging {
							m.currentCall.State = CallStateRinging
							m.broadcastEventLocked("state_change", m.currentCall, "Ringing")
						}
					case 2:
						// 如果已经振铃中或通话中，状态又倒退回 stat=2，说明是基带幽灵残影，电话实际已挂断
						if m.currentCall.State == CallStateRinging || m.currentCall.State == CallStateActive {
							_ = m.hangupLocked("busy")
							m.mu.Unlock()
							return
						}
						if m.currentCall.State != CallStateDialing {
							m.currentCall.State = CallStateDialing
							m.broadcastEventLocked("state_change", m.currentCall, "Dialing...")
						}
					case 6:
						reason := "call_ended"
						if m.currentCall.Direction == DirectionOutbound && (m.currentCall.State == CallStateDialing || m.currentCall.State == CallStateRinging) {
							reason = "busy"
						}
						_ = m.hangupLocked(reason)
						m.mu.Unlock()
						return
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

func cleanPhoneNumber(num string) string {
	var b strings.Builder
	for _, r := range num {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if strings.HasPrefix(s, "86") && len(s) > 2 {
		s = s[2:]
	}
	return s
}

type CLCCEntry struct {
	Index  int
	Dir    int
	Stat   int
	Mode   int
	Number string
}

var clccRegex = regexp.MustCompile(`\+CLCC:\s*(\d+),(\d+),(\d+),(\d+),(\d+)(?:,"([^"]*)")?`)

func parseCLCC(output string) []CLCCEntry {
	var entries []CLCCEntry
	matches := clccRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 6 {
			idx, _ := strconv.Atoi(m[1])
			dir, _ := strconv.Atoi(m[2])
			stat, _ := strconv.Atoi(m[3])
			mode, _ := strconv.Atoi(m[4])
			number := ""
			if len(m) >= 7 {
				number = strings.TrimSpace(m[6])
			}
			if mode == 0 {
				entries = append(entries, CLCCEntry{
					Index:  idx,
					Dir:    dir,
					Stat:   stat,
					Mode:   mode,
					Number: number,
				})
			}
		}
	}
	return entries
}
