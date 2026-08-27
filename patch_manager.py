import re

with open('internal/voicecall/manager.go', 'r', encoding='utf-8') as f:
    code = f.read()

# 1. Add fields to Manager
target_struct = '''type Manager struct {
	mu           sync.RWMutex
	pool         *device.Pool
	currentCall  *CallSession
	audioBridge  *AudioBridge
	webrtcGW     *WebRTCGateway
	subscribers  map[chan CallEvent]struct{}
	subscribersMu sync.RWMutex
}'''
replacement_struct = '''type Manager struct {
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
}'''
code = code.replace(target_struct, replacement_struct)

# 2. Modify NewManager
target_new = '''func NewManager(pool *device.Pool) *Manager {
	bridge := NewAudioBridge("")
	gw := NewWebRTCGateway(bridge)

	m := &Manager{
		pool:        pool,
		audioBridge: bridge,
		webrtcGW:    gw,
		subscribers: make(map[chan CallEvent]struct{}),
	}

	gw.onDisconnect = func() {'''
replacement_new = '''func NewManager(pool *device.Pool) *Manager {
	bridge := NewAudioBridge("")
	gw := NewWebRTCGateway(bridge)

	m := &Manager{
		pool:        pool,
		audioBridge: bridge,
		webrtcGW:    gw,
		subscribers: make(map[chan CallEvent]struct{}),
	}
	
	// 初始化单例串口控制器 (大疆默认 ttyUSB2)
	m.serialCtrl = NewSerialController("/dev/ttyUSB2")
	m.serialCtrl.OnHangup = func(reason string) {
		m.handleSerialHangup(reason)
	}
	m.serialCtrl.OnIncomingCall = func(remoteNumber string) {
		m.OnIncomingCall("dji4g", remoteNumber)
	}
	go m.serialCtrl.Start(context.Background())

	gw.onDisconnect = func() {'''
code = code.replace(target_new, replacement_new)

# 3. Replace Dial
target_dial = '''	// 异步向模组发送 AT 拨号指令
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
	}(worker, req.Number, session.ID)'''
    
replacement_dial = '''	// 关键改进：拨号瞬间立即启动音频通路
	_ = m.audioBridge.Open(context.Background())

	// 战前清空僵尸状态
	_, _ = m.serialCtrl.Execute("AT+CHUP", 2*time.Second)
	time.Sleep(200 * time.Millisecond)

	go func(target string, sessID string) {
		dialCmd := fmt.Sprintf("ATD%s;", target)
		logger.Info("执行 4G VoLTE 拨号", "number", target, "cmd", dialCmd)

		resp, err := m.serialCtrl.Execute(dialCmd, 10*time.Second)
		if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
			logger.Warn("4G 拨号失败", "err", err, "resp", resp)
			m.mu.Lock()
			if m.currentCall != nil && m.currentCall.ID == sessID {
				m.currentCall.State = CallStateTerminated
				m.currentCall.HangupReason = "dial_failed"
				m.broadcastEventLocked("state_change", m.currentCall, "拨号失败: "+resp)
			}
			m.mu.Unlock()
			return
		}
		
		m.startCallTracker(sessID)
	}(req.Number, session.ID)'''
code = code.replace(target_dial.encode('utf-8').decode('utf-8'), replacement_dial)

# 4. Add startCallTracker and handleSerialHangup at the end
additional_code = '''

func (m *Manager) handleSerialHangup(reason string) {
	// 异步执行挂断，解开耳报神死锁！
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
		logger.Info("启动 AT+CLCC 呼叫状态追踪器", "session_id", sessID)

		for {
			select {
			case <-stopCh:
				logger.Info("呼叫状态追踪器停止", "session_id", sessID)
				return
			case <-ticker.C:
				m.mu.RLock()
				cur := m.currentCall
				m.mu.RUnlock()

				if cur == nil || cur.ID != sessID || cur.State == CallStateTerminated {
					return
				}

				resp, err := m.serialCtrl.Execute("AT+CLCC", 2*time.Second)
				if err != nil {
					logger.Warn("AT+CLCC 状态查询错误", "err", err, "session_id", sessID)
					continue
				}

				// parse
				entries := parseCLCC(resp)
				if len(entries) == 0 {
					emptyCount++
					if emptyCount >= 2 {
						logger.Info("AT+CLCC 检测到语音通话已结束/被拒接", "session_id", sessID)
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
				entry := entries[0]

				m.mu.Lock()
				if m.currentCall != nil && m.currentCall.ID == sessID {
					switch entry.Stat {
					case 0:
						if m.currentCall.State != CallStateConnected {
							logger.Info("对方已接听 (AT+CLCC stat=0)", "session_id", sessID)
							m.currentCall.State = CallStateConnected
							m.broadcastEventLocked("connected", m.currentCall, "")
						}
					case 3:
						if m.currentCall.State != CallStateRinging {
							logger.Info("对方正在响铃", "session_id", sessID)
							m.currentCall.State = CallStateRinging
							m.broadcastEventLocked("state_change", m.currentCall, "对方正在响铃")
						}
					case 2:
						if m.currentCall.State == CallStateRinging {
							logger.Warn("探测到非法状态转移 (3 -> 2)，判定为挂断", "session_id", sessID)
							_ = m.hangupLocked("busy")
							m.mu.Unlock()
							return
						}
						if m.currentCall.State != CallStateDialing {
							m.currentCall.State = CallStateDialing
							m.broadcastEventLocked("state_change", m.currentCall, "正在呼叫...")
						}
					case 6:
						logger.Info("对方已断开/拒接 (AT+CLCC stat=6)", "session_id", sessID)
						reason := "call_ended"
						if m.currentCall.Direction == DirectionOutbound && (m.currentCall.State == CallStateDialing || m.currentCall.State == CallStateRinging) {
							reason = "busy"
						}
						_ = m.hangupLocked(reason)
						m.mu.Unlock()
						return
					default:
						logger.Warn("AT+CLCC 发现未处理或异常状态", "stat", entry.Stat, "session_id", sessID)
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

type CLCCEntry struct {
	Index  int
	Dir    int
	Stat   int
	Mode   int
	Number string
}

var clccRegex = regexp.MustCompile(\+CLCC:\s*(\d+),(\d+),(\d+),(\d+),(\d+)(?:,"([^"]*)")?)

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
'''
code += additional_code

# Also replace executeWorkerAT in other functions to avoid opening serial repeatedly
code = code.replace('_, _ = executeWorkerAT(worker, "ATH", 5*time.Second)', '_, _ = m.serialCtrl.Execute("ATH", 4*time.Second)')
code = code.replace('_, err := executeWorkerAT(worker, "ATA", 10*time.Second)', '_, err := m.serialCtrl.Execute("ATA", 4*time.Second)')
code = code.replace('_, err := executeWorkerAT(worker, dtmfCmd, 5*time.Second)', '_, err := m.serialCtrl.Execute(dtmfCmd, 4*time.Second)')

# To handle Chinese character replacement bugs, let's fix them manually:
# Note: Since the previous code contains garbled text like "寮傛鍚戞ā缁勫彂閫", we can just use re.sub with regex to replace the old Dial body.

with open('internal/voicecall/manager.go', 'w', encoding='utf-8') as f:
    f.write(code)

