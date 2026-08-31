package voicecall

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
)

// AudioSource 音频输入流接口（下行：从模组读取声音）
type AudioSource interface {
	io.Reader
	io.Closer
}

// AudioSink 音频输出流接口（上行：向模组写入麦克风声音）
type AudioSink interface {
	io.Writer
	io.Closer
}

// AudioBridge 管理与底层模组的音频输入输出通道
type AudioBridge struct {
	mu          sync.Mutex
	source      AudioSource
	sink        AudioSink
	recCmd      *exec.Cmd
	playCmd     *exec.Cmd
	alsaDevice  string
	cancelFunc  context.CancelFunc
}

func NewAudioBridge(pcmSocketURL string) *AudioBridge {
	return &AudioBridge{
		alsaDevice: "hw:0,0", // 默认大疆 4G 模组 USB-Audio 声卡
	}
}

// detectALSADevice 自动探测模组声卡号
func detectALSADevice() string {
	cmd := exec.Command("arecord", "-l")
	out, err := cmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Baiwang") || strings.Contains(line, "EC25") || strings.Contains(line, "USB Audio") {
				// 解析 card X
				fields := strings.Fields(line)
				for i, f := range fields {
					if f == "card" && i+1 < len(fields) {
						cardNum := strings.TrimRight(fields[i+1], ":")
						devStr := fmt.Sprintf("hw:%s,0", cardNum)
						logger.Info("检测到模组 ALSA 声卡设备", "device", devStr, "info", line)
						return devStr
					}
				}
			}
		}
	}
	return "hw:0,0"
}

// stopCmdSync 同步终止进程并等待其释放声卡句柄
func stopCmdSync(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		logger.Warn("等待 ALSA 进程退出超时", "pid", cmd.Process.Pid)
	}
}

// Open 打开音频通路：启动 arecord 下行采集 与 aplay 上行放音
func (ab *AudioBridge) Open(ctx context.Context) error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// 1. 彻底清理旧资源并等待进程完全退出，释放 ALSA 物理句柄
	if ab.cancelFunc != nil {
		ab.cancelFunc()
		ab.cancelFunc = nil
	}
	if ab.source != nil {
		_ = ab.source.Close()
		ab.source = nil
	}
	if ab.sink != nil {
		_ = ab.sink.Close()
		ab.sink = nil
	}
	if ab.recCmd != nil {
		stopCmdSync(ab.recCmd)
		ab.recCmd = nil
	}
	if ab.playCmd != nil {
		stopCmdSync(ab.playCmd)
		ab.playCmd = nil
	}

	ctx, cancel := context.WithCancel(ctx)
	ab.cancelFunc = cancel

	dev := detectALSADevice()
	ab.alsaDevice = dev

	logger.Info("AudioBridge 正在启动 ALSA 双向音频管道", "alsa_device", dev)

	// 1. 启动 arecord 录音子进程 (带轻度重试与存活检测)
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.CommandContext(ctx, "arecord", "-D", dev, "-f", "S16_LE", "-r", "8000", "-c", "1", "-t", "raw", "--period-time=20000", "--buffer-time=160000")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logger.Warn("创建 arecord 管道失败", "attempt", attempt, "err", err)
		} else if err := cmd.Start(); err != nil {
			logger.Warn("启动 arecord 录音进程失败", "attempt", attempt, "err", err)
			time.Sleep(100 * time.Millisecond)
		} else {
			// 短暂等待检测进程是否由于设备占用而立即夭折
			time.Sleep(50 * time.Millisecond)
			if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
				logger.Warn("arecord 启动后立即退出，重试", "attempt", attempt)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			ab.recCmd = cmd
			ab.source = stdout
			logger.Info("arecord 下行录音进程已就绪", "pid", cmd.Process.Pid, "attempt", attempt)
			break
		}
	}

	// 2. 启动 aplay 放音子进程 (带轻度重试与存活检测)
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.CommandContext(ctx, "aplay", "-D", dev, "-f", "S16_LE", "-r", "8000", "-c", "1", "-t", "raw", "--period-time=20000", "--buffer-time=160000")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			logger.Warn("创建 aplay 管道失败", "attempt", attempt, "err", err)
		} else if err := cmd.Start(); err != nil {
			logger.Warn("启动 aplay 放音进程失败", "attempt", attempt, "err", err)
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(50 * time.Millisecond)
			if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
				logger.Warn("aplay 启动后立即退出，重试", "attempt", attempt)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			ab.playCmd = cmd
			ab.sink = stdin
			logger.Info("aplay 上行放音进程已就绪", "pid", cmd.Process.Pid, "attempt", attempt)
			break
		}
	}

	return nil
}

// ReadPCM 读取模组传来的 PCM 数据
func (ab *AudioBridge) ReadPCM(buf []byte) (int, error) {
	ab.mu.Lock()
	src := ab.source
	ab.mu.Unlock()

	if src == nil {
		return 0, io.EOF
	}
	return src.Read(buf)
}

// WritePCM 将 WebRTC 收到的麦克风 PCM 写入模组声卡
func (ab *AudioBridge) WritePCM(buf []byte) (int, error) {
	ab.mu.Lock()
	snk := ab.sink
	ab.mu.Unlock()

	if snk == nil {
		return 0, nil
	}
	return snk.Write(buf)
}

// Close 关闭音频通路与子进程
func (ab *AudioBridge) Close() error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	logger.Info("AudioBridge 正在关闭音频管道与 ALSA 进程")

	if ab.cancelFunc != nil {
		ab.cancelFunc()
		ab.cancelFunc = nil
	}

	if ab.source != nil {
		_ = ab.source.Close()
		ab.source = nil
	}
	if ab.sink != nil {
		_ = ab.sink.Close()
		ab.sink = nil
	}

	if ab.recCmd != nil {
		stopCmdSync(ab.recCmd)
		ab.recCmd = nil
	}
	if ab.playCmd != nil {
		stopCmdSync(ab.playCmd)
		ab.playCmd = nil
	}

	return nil
}
