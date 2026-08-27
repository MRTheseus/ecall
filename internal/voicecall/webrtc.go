package voicecall

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// WebRTCGateway 管理与前端浏览器的 WebRTC 音频连接
type WebRTCGateway struct {
	mu           sync.Mutex
	peerConn     *webrtc.PeerConnection
	audioTrack   *webrtc.TrackLocalStaticSample
	audioBridge  *AudioBridge
	ctx          context.Context
	cancel       context.CancelFunc
	onICEGather  func(candidate *webrtc.ICECandidate)
	onDisconnect func()

	// ffmpeg 转码进程
	downEncoder  *exec.Cmd // PCM→Opus (下行: 模组声卡 → 浏览器)
	upDecoder    *exec.Cmd // Opus→PCM (上行: 浏览器 → 模组声卡)
	downOpusOut  io.ReadCloser
	upPCMOut     io.ReadCloser
	upOpusIn     io.WriteCloser
	oggWriter    *oggwriter.OggWriter
}

func NewWebRTCGateway(bridge *AudioBridge) *WebRTCGateway {
	return &WebRTCGateway{
		audioBridge: bridge,
	}
}

// HandleOffer 处理前端发来的 SDP Offer 并生成 SDP Answer
func (gw *WebRTCGateway) HandleOffer(ctx context.Context, offerSDP string) (string, error) {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	// 1. 如果已有旧连接，先清理
	if gw.cancel != nil {
		gw.cancel()
	}
	if gw.peerConn != nil {
		_ = gw.peerConn.Close()
	}
	gw.stopTranscoders()

	gw.ctx, gw.cancel = context.WithCancel(context.Background())

	// 2. 创建 WebRTC 媒体引擎与 API
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return "", fmt.Errorf("register default codecs: %w", err)
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetIncludeLoopbackCandidate(true)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	// 3. 创建 PeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		},
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return "", fmt.Errorf("create peer connection: %w", err)
	}
	gw.peerConn = pc

	// 4. 创建下行发送给前端的 Audio Track (Opus 48000Hz)
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"vohive-voice",
	)
	if err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("create audio track: %w", err)
	}
	gw.audioTrack = track

	if _, err := pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("add track to pc: %w", err)
	}

	// 5. 监听前端上行的音频轨（浏览器麦克风声音）
	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		logger.Info("WebRTC 收到前端音频上行 Track", "mime", remoteTrack.Codec().MimeType)
		go gw.handleIncomingAudio(remoteTrack)
	})

	// 6. 连接状态监听
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Info("WebRTC 连接状态变更", "state", state.String())
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			if gw.onDisconnect != nil {
				gw.onDisconnect()
			}
		}
	})

	// 7. 设置 Remote Description
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("set remote description: %w", err)
	}

	// 8. 创建 Answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("create answer: %w", err)
	}

	// 9. 设置 Local Description
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return "", fmt.Errorf("set local description: %w", err)
	}

	// 等待 ICE 候选收集（或超时）
	select {
	case <-gatherComplete:
	case <-time.After(1500 * time.Millisecond):
		logger.Debug("ICE 收集等待超时，返回现有 Answer")
	case <-gw.ctx.Done():
		return "", gw.ctx.Err()
	}

	currentAnswer := pc.LocalDescription()
	if currentAnswer == nil {
		currentAnswer = &answer
	}

	// 10. 启动 ffmpeg 转码管道与下行推流协程
	gw.startTranscoders()
	go gw.startOutgoingAudioStream()

	return currentAnswer.SDP, nil
}

// startTranscoders 启动 ffmpeg 转码进程
func (gw *WebRTCGateway) startTranscoders() {
	ctx := gw.ctx

	// 下行编码器: PCM 8kHz S16_LE → Opus (读取 AudioBridge → ffmpeg → Opus 帧)
	// ffmpeg 从 stdin 读取原始 PCM，输出 OGG/Opus 到 stdout
	gw.downEncoder = exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		"-c:a", "libopus", "-ar", "48000", "-ac", "1",
		"-b:a", "24000",
		"-application", "voip",
		"-frame_duration", "20",
		"-f", "opus", "pipe:1",
	)
	downIn, err := gw.downEncoder.StdinPipe()
	if err != nil {
		logger.Warn("创建下行编码器 stdin 管道失败", "err", err)
		return
	}
	downOut, err := gw.downEncoder.StdoutPipe()
	if err != nil {
		logger.Warn("创建下行编码器 stdout 管道失败", "err", err)
		return
	}
	if err := gw.downEncoder.Start(); err != nil {
		logger.Warn("启动下行 ffmpeg 编码器失败", "err", err)
		return
	}
	gw.downOpusOut = downOut
	logger.Info("下行 ffmpeg Opus 编码器已启动", "pid", gw.downEncoder.Process.Pid)

	// 启动后台协程: 从 AudioBridge 读取 PCM 并喂给 ffmpeg 编码器
	go func() {
		pcmBuf := make([]byte, 320) // 8kHz 16-bit 20ms = 320 bytes
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := gw.audioBridge.ReadPCM(pcmBuf)
				if err != nil {
					if err == io.EOF || ctx.Err() != nil {
						return
					}
					time.Sleep(5 * time.Millisecond)
					continue
				}
				if n > 0 {
					_, writeErr := downIn.Write(pcmBuf[:n])
					if writeErr != nil {
						return
					}
				}
			}
		}
	}()

	// 上行解码器: Opus → PCM 8kHz S16_LE (浏览器麦克风 Opus → ffmpeg → AudioBridge)
	gw.upDecoder = exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-c:a", "libopus",
		"-f", "ogg", "-i", "pipe:0",
		"-f", "s16le", "-ar", "8000", "-ac", "1", "pipe:1",
	)
	upIn, err := gw.upDecoder.StdinPipe()
	if err != nil {
		logger.Warn("创建上行解码器 stdin 管道失败", "err", err)
		return
	}
	upOut, err := gw.upDecoder.StdoutPipe()
	if err != nil {
		logger.Warn("创建上行解码器 stdout 管道失败", "err", err)
		return
	}
	if err := gw.upDecoder.Start(); err != nil {
		logger.Warn("启动上行 ffmpeg 解码器失败", "err", err)
		return
	}
	gw.upOpusIn = upIn
	gw.upPCMOut = upOut
	writer, err := oggwriter.NewWith(upIn, 48000, 1)
	if err != nil {
		logger.Warn("创建 oggwriter 失败", "err", err)
	} else {
		gw.oggWriter = writer
	}
	logger.Info("上行 ffmpeg Opus 解码器已启动", "pid", gw.upDecoder.Process.Pid)

	// 启动后台协程: 从 ffmpeg 解码器读取 PCM 并写入 AudioBridge (模组声卡)
	go func() {
		pcmBuf := make([]byte, 320)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := upOut.Read(pcmBuf)
				if err != nil {
					return
				}
				if n > 0 {
					_, _ = gw.audioBridge.WritePCM(pcmBuf[:n])
				}
			}
		}
	}()
}

// stopTranscoders 停止 ffmpeg 转码进程
func (gw *WebRTCGateway) stopTranscoders() {
	gw.oggWriter = nil
	if gw.downOpusOut != nil {
		_ = gw.downOpusOut.Close()
		gw.downOpusOut = nil
	}
	if gw.upOpusIn != nil {
		_ = gw.upOpusIn.Close()
		gw.upOpusIn = nil
	}
	if gw.upPCMOut != nil {
		_ = gw.upPCMOut.Close()
		gw.upPCMOut = nil
	}
	if gw.downEncoder != nil && gw.downEncoder.Process != nil {
		_ = gw.downEncoder.Process.Kill()
		gw.downEncoder = nil
	}
	if gw.upDecoder != nil && gw.upDecoder.Process != nil {
		_ = gw.upDecoder.Process.Kill()
		gw.upDecoder = nil
	}
}

// AddICECandidate 添加前端发来的 ICE 候选
func (gw *WebRTCGateway) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	gw.mu.Lock()
	pc := gw.peerConn
	gw.mu.Unlock()

	if pc == nil {
		return fmt.Errorf("peer connection not ready")
	}
	return pc.AddICECandidate(candidate)
}

// handleIncomingAudio 处理浏览器发来的麦克风音频流 (Opus RTP → oggwriter → ffmpeg → PCM → aplay)
func (gw *WebRTCGateway) handleIncomingAudio(remoteTrack *webrtc.TrackRemote) {
	for {
		select {
		case <-gw.ctx.Done():
			return
		default:
			pkt, _, err := remoteTrack.ReadRTP()
			if err != nil {
				if err == io.EOF {
					return
				}
				logger.Debug("读取上行 RTP 数据包失败", "err", err)
				return
			}
			gw.mu.Lock()
			writer := gw.oggWriter
			gw.mu.Unlock()
			if writer != nil && len(pkt.Payload) > 0 {
				_ = writer.WriteRTP(pkt)
			}
		}
	}
}

// startOutgoingAudioStream 从 ffmpeg 编码器读取 OggOpus，解包出裸 Opus 帧并发送给浏览器
func (gw *WebRTCGateway) startOutgoingAudioStream() {
	if gw.downOpusOut == nil || gw.audioTrack == nil {
		return
	}

	ogg, _, err := oggreader.NewWith(gw.downOpusOut)
	if err != nil {
		logger.Warn("初始化 oggreader 失败", "err", err)
		return
	}

	for {
		select {
		case <-gw.ctx.Done():
			return
		default:
			pageData, _, err := ogg.ParseNextPage()
			if err != nil {
				if err == io.EOF || gw.ctx.Err() != nil {
					return
				}
				time.Sleep(5 * time.Millisecond)
				continue
			}
			if len(pageData) == 0 {
				continue
			}

			err = gw.audioTrack.WriteSample(media.Sample{
				Data:     pageData,
				Duration: 20 * time.Millisecond,
			})
			if err != nil && err != io.ErrClosedPipe {
				logger.Debug("向 WebRTC 写入音频帧失败", "err", err)
			}
		}
	}
}

// Close 关闭 WebRTC 连接与相关协程
func (gw *WebRTCGateway) Close() {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	if gw.cancel != nil {
		gw.cancel()
	}
	gw.stopTranscoders()
	if gw.peerConn != nil {
		_ = gw.peerConn.Close()
		gw.peerConn = nil
	}
	gw.audioTrack = nil
}
