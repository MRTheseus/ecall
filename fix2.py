import re

with open('internal/voicecall/manager.go', 'r', encoding='utf-8', errors='ignore') as f:
    lines = f.readlines()

for i, line in enumerate(lines):
    # Count quotes
    if line.count('"') % 2 != 0:
        line = line.rstrip('\r\n') + '"\n'
    
    # If line is missing closing parens for some function calls, add them
    if 'logger.' in line and '(' in line and ')' not in line:
        line = line.rstrip('\r\n') + ')\n'
        
    if 'fmt.Errorf' in line and '(' in line and ')' not in line:
        line = line.rstrip('\r\n') + ')\n'

    # Special specific fixes
    if 'logger.Info("WebRTC' in line and ')' not in line:
        line = '\t\tlogger.Info("WebRTC disconnected")\n'
    if 'fmt.Errorf("打开' in line:
        line = '\t\treturn "", fmt.Errorf("open port failed: %w", err)\n'
    if 'logger.Warn("启动' in line:
        line = '\t\tlogger.Warn("start failed", "err", err)\n'
    if 'logger.Warn("AT+CLCC' in line:
        line = '\t\t\t\t\tlogger.Warn("AT+CLCC error", "err", err, "session_id", sessID)\n'

    lines[i] = line

with open('internal/voicecall/manager.go', 'w', encoding='utf-8') as f:
    f.writelines(lines)
