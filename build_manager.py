import re
with open('internal/voicecall/manager.go', 'r', encoding='utf-8') as f:
    code = f.read()

# 1. Add fields to Manager
code = code.replace(
'''type Manager struct {
	mu           sync.RWMutex
	pool         *device.Pool
	currentCall  *CallSession
	audioBridge  *AudioBridge
	webrtcGW     *WebRTCGateway
	subscribers  map[chan CallEvent]struct{}
	subscribersMu sync.RWMutex
}''',
'''type Manager struct {
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
)

# 2. Modify NewManager
code = code.replace(
'''func NewManager(pool *device.Pool) *Manager {
	bridge := NewAudioBridge("")
	gw := NewWebRTCGateway(bridge)

	m := &Manager{
		pool:        pool,
		audioBridge: bridge,
		webrtcGW:    gw,
		subscribers: make(map[chan CallEvent]struct{}),
	}''',
'''func NewManager(pool *device.Pool) *Manager {
	bridge := NewAudioBridge("")
	gw := NewWebRTCGateway(bridge)

	m := &Manager{
		pool:        pool,
		audioBridge: bridge,
		webrtcGW:    gw,
		subscribers: make(map[chan CallEvent]struct{}),
	}
	m.serialCtrl = NewSerialController("/dev/ttyUSB2")
	m.serialCtrl.OnHangup = func(reason string) {
		m.handleSerialHangup(reason)
	}
	m.serialCtrl.OnIncomingCall = func(remoteNumber string) {
		m.OnIncomingCall("dji4g", remoteNumber)
	}
	go m.serialCtrl.Start(context.Background())'''
)

# 3. Replace Dial
code = re.sub(r'go func\(dev \*device\.Worker, target string, sessID string\) \{.*?\n\t\}\(worker, req\.Number, session\.ID\)',
'''_ = m.audioBridge.Open(context.Background())
	_, _ = m.serialCtrl.Execute("AT+CHUP", 2*time.Second)
	time.Sleep(200 * time.Millisecond)

	go func(target string, sessID string) {
		dialCmd := fmt.Sprintf("ATD%s;", target)
		resp, err := m.serialCtrl.Execute(dialCmd, 10*time.Second)
		if err != nil || (!strings.Contains(resp, "OK") && !strings.Contains(resp, "CONNECT")) {
			m.mu.Lock()
			if m.currentCall != nil && m.currentCall.ID == sessID {
				m.currentCall.State = CallStateTerminated
				m.currentCall.HangupReason = "dial_failed"
				m.broadcastEventLocked("state_change", m.currentCall, "Dial failed: " + resp)
			}
			m.mu.Unlock()
			return
		}
		m.startCallTracker(sessID)
	}(req.Number, session.ID)''', code, flags=re.DOTALL)

# 4. other replaces
code = code.replace('_, _ = executeWorkerAT(worker, "ATH", 5*time.Second)', '_, _ = m.serialCtrl.Execute("ATH", 4*time.Second)')
code = code.replace('_, err := executeWorkerAT(worker, "ATA", 10*time.Second)', '_, err := m.serialCtrl.Execute("ATA", 4*time.Second)')
code = code.replace('_, err := executeWorkerAT(worker, dtmfCmd, 5*time.Second)', '_, err := m.serialCtrl.Execute(dtmfCmd, 4*time.Second)')

additional_code = '''
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

				resp, err := m.serialCtrl.Execute("AT+CLCC", 2*time.Second)
				if err != nil {
					continue
				}

				entries := parseCLCC(resp)
				if len(entries) == 0 {
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
				entry := entries[0]

				m.mu.Lock()
				if m.currentCall != nil && m.currentCall.ID == sessID {
					switch entry.Stat {
					case 0:
						if m.currentCall.State != CallStateConnected {
							m.currentCall.State = CallStateConnected
							m.broadcastEventLocked("connected", m.currentCall, "")
						}
					case 3:
						if m.currentCall.State != CallStateRinging {
							m.currentCall.State = CallStateRinging
							m.broadcastEventLocked("state_change", m.currentCall, "Ringing")
						}
					case 2:
						if m.currentCall.State == CallStateRinging {
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

with open('internal/voicecall/manager.go', 'w', encoding='utf-8') as f:
    f.write(code)
