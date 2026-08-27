with open('internal/voicecall/manager.go', 'a', encoding='utf-8') as f:
    f.write('''
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

var clccRegex = regexp.MustCompile(`\\+CLCC:\\s*(\\d+),(\\d+),(\\d+),(\\d+),(\\d+)(?:,"([^"]*)")?`)

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
''')
