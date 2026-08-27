import re

with open('internal/voicecall/manager.go', 'r', encoding='utf-8') as f:
    code = f.read()

target = '''func (m *Manager) handleSerialHangup(rawReason string) {
	m.mu.Lock()
	defer m.mu.Unlock()'''
replacement = '''func (m *Manager) handleSerialHangup(rawReason string) {
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()'''
code = code.replace(target, replacement)

target2 = '''	if m.currentCall != nil {
		m.broadcastEventLocked("hangup", m.currentCall, "通话结束(" + reason + ")")
	}
}'''
replacement2 = '''	if m.currentCall != nil {
		m.broadcastEventLocked("hangup", m.currentCall, "通话结束(" + reason + ")")
	}
	}()
}'''
code = code.replace(target2, replacement2)

with open('internal/voicecall/manager.go', 'w', encoding='utf-8') as f:
    f.write(code)
