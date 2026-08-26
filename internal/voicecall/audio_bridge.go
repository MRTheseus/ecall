package voicecall

import (
	"context"
	"io"
	"net"
	"os"
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
	mu           sync.Mutex
	source       AudioSource
	sink         AudioSink
	isFallback   bool
	pcmSocketURL string
}

func NewAudioBridge(pcmSocketURL string) *AudioBridge {
	return &AudioBridge{
		pcmSocketURL: pcmSocketURL,
	}
}

// Open 打开音频通路（优先尝试 Socket 或 ALSA 文件，失败则进入安全回退模式）
func (ab *AudioBridge) Open(ctx context.Context) error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// 1. 如果配置了 PCM Socket 地址（如 127.0.0.1:9000），尝试 TCP 连接
	if ab.pcmSocketURL != "" {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", ab.pcmSocketURL)
		if err == nil {
			logger.Info("AudioBridge 已连接至 PCM Socket", "addr", ab.pcmSocketURL)
			ab.source = conn
			ab.sink = conn
			ab.isFallback = false
			return nil
		}
		logger.Warn("AudioBridge 连接 PCM Socket 失败，尝试本地声卡", "err", err)
	}

	// 2. 尝试打开 Linux ALSA 原始 PCM 设备文件节点（若存在）
	alsaCapPaths := []string{"/dev/snd/pcmC1D0c", "/dev/snd/pcmC2D0c", "/dev/snd/pcmC0D0c"}
	for _, p := range alsaCapPaths {
		if f, err := os.OpenFile(p, os.O_RDWR, 0666); err == nil {
			logger.Info("AudioBridge 打开 ALSA 设备成功", "path", p)
			ab.source = f
			ab.sink = f
			ab.isFallback = false
			return nil
		}
	}

	// 3. Fallback 安全模式：创建内存管道（静音/自环保活）
	logger.Info("AudioBridge 进入 Fallback 虚拟音频流模式")
	r, w := io.Pipe()
	ab.source = r
	ab.sink = w
	ab.isFallback = true

	// 启动后台静音/心跳帧发生器，避免 WebRTC 接收端超时无声
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond) // 20ms 一帧
		defer ticker.Stop()
		silenceFrame := make([]byte, 320) // 8000Hz 16-bit 20ms = 160 samples = 320 bytes
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ab.isFallback && ab.sink != nil {
					_, _ = ab.sink.Write(silenceFrame)
				}
			}
		}
	}()

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

// WritePCM 将 WebRTC 收到的麦克风 PCM 写入模组
func (ab *AudioBridge) WritePCM(buf []byte) (int, error) {
	ab.mu.Lock()
	snk := ab.sink
	ab.mu.Unlock()

	if snk == nil {
		return 0, nil
	}
	return snk.Write(buf)
}

// Close 关闭音频通道
func (ab *AudioBridge) Close() error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.source != nil {
		_ = ab.source.Close()
		ab.source = nil
	}
	if ab.sink != nil {
		_ = ab.sink.Close()
		ab.sink = nil
	}
	return nil
}
