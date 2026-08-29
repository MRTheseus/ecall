package voicecall

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
	"go.bug.st/serial"
)

var (
	clipRegex = regexp.MustCompile(`\+CLIP:\s*"([^"]+)"`)
)

// SerialController 单例持久化串口调度器，常驻持有串口并集中处理 URC 与同步指令
type SerialController struct {
	portPath string
	baudRate int

	mu        sync.Mutex // 保护同步指令执行
	port      serial.Port
	reader    *bufio.Reader
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.Mutex

	// 事件回调函数
	OnIncoming func(remoteNumber string)
	OnHangup   func(reason string)
	OnRing     func()

	// 用于同步 AT 指令的响应通道
	cmdRespCh chan string
	isWaitingCmd bool
}

func NewSerialController(portPath string, baudRate int) *SerialController {
	if portPath == "" {
		portPath = "/dev/ttyUSB3"
	}
	if baudRate <= 0 {
		baudRate = 115200
	}
	return &SerialController{
		portPath:  portPath,
		baudRate:  baudRate,
		cmdRespCh: make(chan string, 16),
	}
}

// Start 启动持久化串口监听
func (sc *SerialController) Start(ctx context.Context) error {
	sc.runningMu.Lock()
	defer sc.runningMu.Unlock()

	if sc.running {
		return nil
	}

	mode := &serial.Mode{
		BaudRate: sc.baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(sc.portPath, mode)
	if err != nil {
		return fmt.Errorf("打开串口 %s 失败: %w", sc.portPath, err)
	}

	sc.port = p
	sc.reader = bufio.NewReader(p)
	sc.ctx, sc.cancel = context.WithCancel(ctx)
	sc.running = true

	// 启动后台常驻读取协程
	go sc.listenLoop()

	// 初始化基带报告配置
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = sc.Execute("ATE0", 2*time.Second)       // 关闭回显
		_, _ = sc.Execute("AT+CLIP=1", 2*time.Second) // 开启来电显示
		_, _ = sc.Execute("AT+CRC=1", 2*time.Second)  // 开启扩展呼叫结果码
		_, _ = sc.Execute("ATX4", 2*time.Second)      // 开启忙音检测与详细结果码
		logger.Info("单例串口调度器初始化完毕", "port", sc.portPath)
	}()

	return nil
}

// Stop 停止调度器并释放串口
func (sc *SerialController) Stop() {
	sc.runningMu.Lock()
	defer sc.runningMu.Unlock()

	if !sc.running {
		return
	}

	sc.running = false
	if sc.cancel != nil {
		sc.cancel()
	}
	if sc.port != nil {
		_ = sc.port.Close()
		sc.port = nil
	}
}

// Execute 同步发送一条 AT 指令并等待其完整响应 (互斥排队)
func (sc *SerialController) Execute(cmd string, timeout time.Duration) (string, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.port == nil || !sc.running {
		return "", fmt.Errorf("串口调度器未就绪")
	}

	// 清空历史通道残留
	for len(sc.cmdRespCh) > 0 {
		<-sc.cmdRespCh
	}

	sc.isWaitingCmd = true
	defer func() {
		sc.isWaitingCmd = false
		for len(sc.cmdRespCh) > 0 {
			<-sc.cmdRespCh
		}
	}()

	// 写入指令
	cmdStr := strings.TrimSpace(cmd) + "\r\n"
	if _, err := sc.port.Write([]byte(cmdStr)); err != nil {
		return "", fmt.Errorf("写入 AT 指令失败: %w", err)
	}

	var respLines []string
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return strings.Join(respLines, "\r\n"), fmt.Errorf("AT 指令超时 (%v): %s", timeout, cmd)
		case <-sc.ctx.Done():
			return strings.Join(respLines, "\r\n"), sc.ctx.Err()
		case line, ok := <-sc.cmdRespCh:
			if !ok {
				return strings.Join(respLines, "\r\n"), fmt.Errorf("串口已关闭")
			}
			respLines = append(respLines, line)

			// 检查终止符
			if line == "OK" || strings.HasPrefix(line, "ERROR") || strings.HasPrefix(line, "+CME ERROR:") || strings.HasPrefix(line, "+CMS ERROR:") || strings.Contains(line, "BUSY") || strings.Contains(line, "NO CARRIER") || strings.Contains(line, "NO ANSWER") || strings.Contains(line, "CONNECT") {
				return strings.Join(respLines, "\r\n"), nil
			}
		}
	}
}

// listenLoop 常驻串口读取与 URC 解析主循环
func (sc *SerialController) listenLoop() {
	for {
		select {
		case <-sc.ctx.Done():
			return
		default:
			line, err := sc.reader.ReadString('\n')
			if err != nil {
				if sc.ctx.Err() != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// 如果有正在等待响应的同步 AT 指令，优先送入通道
			// 此时 BUSY/NO CARRIER 会作为 Execute 的终止符返回，不再重复触发 URC 回调
			if sc.isWaitingCmd {
				select {
				case sc.cmdRespCh <- line:
				default:
				}
			} else {
				// 只有在空闲状态下才处理异步 URC（避免 BUSY/NO CARRIER 双重触发）
				sc.handleURC(line)
			}
		}
	}
}

func (sc *SerialController) handleURC(line string) {
	// 1. 匹配来电显示 (+CLIP: "189...",128)
	if strings.HasPrefix(line, "+CLIP:") {
		matches := clipRegex.FindStringSubmatch(line)
		remote := "未知号码"
		if len(matches) >= 2 {
			remote = matches[1]
		}
		logger.Info("串口捕获到来电 URC", "number", remote, "raw", line)
		if sc.OnIncoming != nil {
			sc.OnIncoming(remote)
		}
		return
	}

	// 2. 匹配振铃 (RING)
	if line == "RING" {
		if sc.OnRing != nil {
			sc.OnRing()
		}
		return
	}

	// 3. 匹配拒接 / 挂断 / 忙音
	if strings.Contains(line, "BUSY") {
		logger.Info("串口捕获到基带 BUSY 拒接信号", "raw", line)
		if sc.OnHangup != nil {
			sc.OnHangup("busy")
		}
	} else if strings.Contains(line, "NO CARRIER") {
		logger.Info("串口捕获到基带 NO CARRIER 挂断信号", "raw", line)
		if sc.OnHangup != nil {
			sc.OnHangup("no_carrier")
		}
	} else if strings.Contains(line, "NO ANSWER") {
		logger.Info("串口捕获到基带 NO ANSWER 无人接听信号", "raw", line)
		if sc.OnHangup != nil {
			sc.OnHangup("no_answer")
		}
	}
}
