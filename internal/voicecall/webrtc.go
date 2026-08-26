package voicecall

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
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

	gw.ctx, gw.cancel = context.WithCancel(ctx)

	// 2. 创建 WebRTC 媒体引擎与 API
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return "", fmt.Errorf("register default codecs: %w", err)
	}

	settingEngine := webrtc.SettingEngine{}
	// 放行局域网与标准 ICE 候选
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

	// 4. 创建下行发送给前端的 Audio Track (Opus 48000Hz Stereo/Mono)
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

	// 10. 启动下行推流协程（从模组读取声音并推给前端）
	go gw.startOutgoingAudioStream()

	return currentAnswer.SDP, nil
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

// handleIncomingAudio 处理浏览器发来的麦克风音频流
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
			// 将上行数据写入 AudioBridge 传递给模组
			if gw.audioBridge != nil && len(pkt.Payload) > 0 {
				_, _ = gw.audioBridge.WritePCM(pkt.Payload)
			}
		}
	}
}

// startOutgoingAudioStream 从模组读取下行音频并封装发送给浏览器
func (gw *WebRTCGateway) startOutgoingAudioStream() {
	if gw.audioBridge == nil || gw.audioTrack == nil {
		return
	}

	// 每 20ms 发送一个音频 Sample
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	pcmBuf := make([]byte, 320) // 8000Hz 16-bit 20ms PCM 帧
	for {
		select {
		case <-gw.ctx.Done():
			return
		case <-ticker.C:
			n, err := gw.audioBridge.ReadPCM(pcmBuf)
			if err != nil && err != io.EOF {
				continue
			}

			payload := pcmBuf[:n]
			if n == 0 {
				payload = make([]byte, 320) // 静音回填
			}

			// 写入 WebRTC 本地音轨
			err = gw.audioTrack.WriteSample(media.Sample{
				Data:     payload,
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
	if gw.peerConn != nil {
		_ = gw.peerConn.Close()
		gw.peerConn = nil
	}
	gw.audioTrack = nil
}
