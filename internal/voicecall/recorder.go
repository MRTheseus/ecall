package voicecall

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
)

// CallRecorder 服务端双向通话录音引擎 (PCM 混音 -> FFmpeg MP3 实时编码)
type CallRecorder struct {
	mu           sync.Mutex
	sessionID    string
	remoteNumber string
	filePath     string
	relPath      string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	cancel       context.CancelFunc
	running      bool
	startedAt    time.Time

	// 混音缓冲区
	downQueue chan []byte
	upQueue   chan []byte
	stopMixer chan struct{}
}

// NewCallRecorder 创建录音引擎实例
func NewCallRecorder() *CallRecorder {
	return &CallRecorder{}
}

// getRecordingsBaseDir 获取录音存储物理根目录
func getRecordingsBaseDir() string {
	if _, err := os.Stat("/app/data"); err == nil {
		return "/app/data/recordings"
	}
	return "./data/recordings"
}

// Start 启动通话录音
func (r *CallRecorder) Start(sessionID string, remoteNumber string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		logger.Warn("录音已经在进行中", "sessionID", r.sessionID)
		return nil
	}

	baseDir := getRecordingsBaseDir()
	now := time.Now()
	subDir := now.Format("200601")
	dirPath := filepath.Join(baseDir, subDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		logger.Warn("创建录音目录失败", "path", dirPath, "err", err)
		return err
	}

	cleanNum := cleanPhoneNumber(remoteNumber)
	if cleanNum == "" {
		cleanNum = "unknown"
	}
	fileName := fmt.Sprintf("rec_%s_%s_%s.mp3", now.Format("20060102_150405"), cleanNum, sessionID[:8])
	fullPath := filepath.Join(dirPath, fileName)
	relPath := filepath.Join(subDir, fileName)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		"-c:a", "libmp3lame", "-ar", "16000", "-ac", "1", "-b:a", "32k",
		"-y", fullPath,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		logger.Warn("创建录音 FFmpeg stdin 管道失败", "err", err)
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		logger.Warn("启动录音 FFmpeg 进程失败", "err", err)
		return err
	}

	r.sessionID = sessionID
	r.remoteNumber = remoteNumber
	r.filePath = fullPath
	r.relPath = relPath
	r.cmd = cmd
	r.stdin = stdin
	r.running = true
	r.startedAt = now
	r.downQueue = make(chan []byte, 100)
	r.upQueue = make(chan []byte, 100)
	r.stopMixer = make(chan struct{})

	logger.Info("服务端通话录音已启动", "sessionID", sessionID[:8], "file", relPath, "pid", cmd.Process.Pid)

	go r.mixLoop(stdin)

	return nil
}

// PushDownstream 注入下行音频 (对方声音)
func (r *CallRecorder) PushDownstream(pcm []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.downQueue == nil {
		return
	}
	buf := make([]byte, len(pcm))
	copy(buf, pcm)
	select {
	case r.downQueue <- buf:
	default:
	}
}

// PushUpstream 注入上行音频 (我方麦克风声音)
func (r *CallRecorder) PushUpstream(pcm []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.upQueue == nil {
		return
	}
	buf := make([]byte, len(pcm))
	copy(buf, pcm)
	select {
	case r.upQueue <- buf:
	default:
	}
}

// mixLoop 混音主循环
func (r *CallRecorder) mixLoop(out io.WriteCloser) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	silence := make([]byte, 320)

	for {
		select {
		case <-r.stopMixer:
			return
		case <-ticker.C:
			var down []byte
			var up []byte

			select {
			case down = <-r.downQueue:
			default:
			}

			select {
			case up = <-r.upQueue:
			default:
			}

			if len(down) == 0 && len(up) == 0 {
				continue
			}

			if len(down) == 0 {
				down = silence
			}
			if len(up) == 0 {
				up = silence
			}

			mixed := mixS16LE(down, up)
			_, err := out.Write(mixed)
			if err != nil {
				return
			}
		}
	}
}

// mixS16LE 16-bit PCM 线性叠加混音带防爆音限幅截断
func mixS16LE(a, b []byte) []byte {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	n -= n % 2
	mixed := make([]byte, n)

	for i := 0; i < n; i += 2 {
		s1 := int32(int16(binary.LittleEndian.Uint16(a[i:])))
		s2 := int32(int16(binary.LittleEndian.Uint16(b[i:])))
		sum := s1 + s2
		if sum > 32767 {
			sum = 32767
		} else if sum < -32768 {
			sum = -32768
		}
		binary.LittleEndian.PutUint16(mixed[i:], uint16(int16(sum)))
	}
	return mixed
}

// Stop 停止录音并封包落盘
func (r *CallRecorder) Stop() (string, int64, error) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return "", 0, nil
	}

	r.running = false
	close(r.stopMixer)
	if r.stdin != nil {
		_ = r.stdin.Close()
	}

	cmd := r.cmd
	filePath := r.filePath
	relPath := r.relPath
	cancel := r.cancel
	r.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		if cmd != nil {
			done <- cmd.Wait()
		} else {
			done <- nil
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if cancel != nil {
			cancel()
		}
	}

	var size int64
	if fi, err := os.Stat(filePath); err == nil {
		size = fi.Size()
	}

	logger.Info("服务端通话录音已完成落盘", "relPath", relPath, "sizeBytes", size)
	return relPath, size, nil
}
