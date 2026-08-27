import re
with open('/root/vohive_src/internal/voicecall/manager.go', 'r', encoding='utf-8', errors='ignore') as f:
    lines = f.readlines()
for i in range(len(lines)):
    line = lines[i]
    if line.count('"') % 2 != 0:
        line = line.rstrip('\r\n') + '"\n'
    if 'logger.' in line and '(' in line and ')' not in line:
        line = line.rstrip('\r\n') + ')\n'
    if 'fmt.Errorf' in line and '(' in line and ')' not in line:
        line = line.rstrip('\r\n') + ')\n'
    if 'logger.Info("WebRTC' in line and ')' not in line:
        line = '\t\tlogger.Info("WebRTC disconnected")\n'
    lines[i] = line
with open('/root/vohive_src/internal/voicecall/manager.go', 'w', encoding='utf-8') as f:
    f.writelines(lines)
