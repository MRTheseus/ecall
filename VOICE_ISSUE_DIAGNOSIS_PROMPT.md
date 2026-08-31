# VoHive 蜂窝语音呼叫【下行音频中断 / 我方听不到对方】深度诊断与架构分析 Prompt

> **使用说明**：本 Prompt 专为大语言模型（如 Claude 3.5 Sonnet / GPT-4o / Gemini 1.5 Pro / DeepSeek-R1 等）设计，包含了 **VoHive 蜂窝语音通信系统** 的完整架构背景、双向音频流水线、历史排查轨迹以及核心源码。请直接将本 Prompt 复制给目标模型进行深度代码审计与根因分析。

---

## 🎯 任务背景与核心故障描述

你是一位精通 **Linux 音频架构 (ALSA/UAC)、实时音视频通信 (WebRTC/Pion)、FFmpeg 转码流水线、Go 并发编程以及 3GPP 蜂窝模组 (Quectel VoLTE/AT 指令/早媒体)** 的资深架构师。

我们在开源项目 **VoHive**（一个基于 Linux + 4G 模组声卡 + WebRTC 的蜂窝电话网关系统）中遇到了一个极为顽固的音频链路缺陷：

### 🚨 核心故障现象
1. **上行音频完全正常（对方能听清我方）**：浏览器端麦克风说话，经 WebRTC 传输至服务端转码后写入 4G 模组声卡，对方手机听筒能够清晰听到我方声音。
2. **下行音频完全无声 / 严重中断（我方听不到对方）**：
   - 无论是通话正式接通后对方说话，还是呼叫过程中的运营商早期媒体提示音（如：“您拨打的电话正在通话中”、“用户正忙”），**Web 端浏览器均听不到任何声音（静音或未收到音频流）**；
   - 在开发过程中，下行音频曾经过多次修补（增加了静音帧预热、保活补偿、重试机制等），但经常出现“首次或某次能听到，挂断再呼叫后下行再次彻底瘫痪”或“下行推流协程静默退出”的问题。

请对系统架构、音频流转逻辑、并发时序和相关源码进行**全链路深度诊断**，找出导致下行音频中断的根本原因，并给出彻底、健壮的修复方案。

---

## 🏗️ 系统拓扑与双向音频流水线

系统运行于 **Debian Linux 宿主机（Docker 特权容器，`network_mode: host`）**，硬件搭载 **Quectel EC25 / 大疆 4G 模组（USB ID `2c7c:0125`，提供 USB UAC 声卡 `hw:0,0` 与 AT 串口 `/dev/ttyUSB2`）**。

### 1. 全双工音频数据流向拓扑图

```text
==============================================================================================================
【上行链路 (正常通畅)】: 浏览器麦克风 ➔ 对方手机听筒
  [Browser 麦克风] 
      │ (Opus 48kHz RTP)
      ▼
  [Pion WebRTC Gateway] ➔ 提取裸 Opus 载荷 ➔ oggwriter ➔ pipe 
      │ 
      ▼
  [FFmpeg upDecoder] (-c:a libopus -f ogg -i pipe:0 ➔ -f s16le -ar 8000 -ac 1)
      │ (PCM 8kHz 16-bit Mono)
      ▼
  [AudioBridge.WritePCM] ➔ [aplay -D hw:0,0] ➔ [4G 模组 UAC 声卡 DAC] ➔ [VoLTE 基带] ➔ [对方手机]

==============================================================================================================
【下行链路 (当前故障：无声/中断)】: 对方手机 / 运营商早媒体 ➔ 浏览器听筒
  [对方手机声音 / 运营商提示音] 
      │ (4G VoLTE 下行)
      ▼
  [4G 模组 UAC 声卡 ADC] ➔ [arecord -D hw:0,0 -f S16_LE -r 8000 -c 1] 
      │ (PCM 8kHz 16-bit Mono)
      ▼
  [AudioBridge.ReadPCM] (Go io.Pipe / buffer)
      │ 
      ▼
  [FFmpeg downEncoder] (-f s16le -ar 8000 -ac 1 -i pipe:0 ➔ -c:a libopus -f opus -ar 48000 -ac 1 pipe:1)
      │ (Ogg/Opus 封装流)
      ▼
  [Pion oggreader.NewWith(downOpusOut)] ➔ [oggreader.ParseNextPage()]
      │ (Opus 音频帧 Page Payload)
      ▼
  [WebRTC TrackLocalStaticSample.WriteSample(media.Sample{Data, Duration: 20ms})]
      │ (WebRTC SRTP/RTP)
      ▼
  [Browser pc.ontrack] ➔ [HTMLAudioElement.srcObject = stream] ➔ [扬声器/耳机] (❌ 当前无声)
==============================================================================================================
```

---

## 📜 历史排查轨迹与踩坑复盘（Context & Pitfalls）

为了避免重复无效的排查，以下是此前排查中确认的事实与踩过的坑：

### 1. ALSA 声卡句柄与并发冲突
* 4G 模组提供的是单物理声卡 `hw:0,0`。
* 在 Go 代码中，上行使用 `aplay` 子进程写入，下行使用 `arecord` 子进程读取。如果上一通呼叫挂断时 `arecord` / `aplay` 进程未彻底杀死或 ALSA 句柄未完全释放，下一次呼叫时打开 `hw:0,0` 会遭遇 `Device or resource busy`。为此代码中加入了释放等待与重试。

### 2. FFmpeg 编码器与 Pion `oggreader` 的死锁/时序冲突
* Pion 的 `oggreader.NewWith(reader)` 在初始化时，**必须同步读取并解析出 Ogg Header（包含 `OpusHead` 和 `OpusTags`）**。
* 如果 FFmpeg 启动后，其 stdin 没有立刻被喂入足够的 PCM 数据，FFmpeg 不会向 stdout 输出任何数据；此时 `oggreader.NewWith` 会**永久阻塞或读取到 EOF 报错退出**（报错：`oggreader: file already closed`）。
* 此前在 `webrtc.go` 中尝试在启动 FFmpeg 后立即注入 3 帧静音 PCM（`320 * 3` 字节）作为预热，并用平滑静音帧保活。

### 3. 早期媒体（Early Media）与拒接时序
* 当拨打 10086 或被叫方拒接时，基带会通过下行音频播放运营商的语音提示（如“正在通话中”），但同时模组串口可能已上报 `BUSY` 或 CLCC 状态脱离活跃列表。
* 若系统一收到 `BUSY` 就立刻执行 `hangupLocked` 掐断声卡与 WebRTC，会导致前端完全听不到拒接原因。为此加入了 4 秒延时挂机机制。

### 4. 浏览器 WebRTC 生命周期
* 前端在点击拨号或收到来电时，立刻创建 `RTCPeerConnection`，执行 `POST /api/voice/webrtc/offer` 交换 SDP。
* 前端通过 `audioEl.srcObject = remoteStream` 播放远端流，但在部分呼叫场景下，`ontrack` 事件触发后依然无法听到声音。

---

## 💻 核心源码清单（关键模块）

以下是当前系统中处理音频与呼叫生命周期的核心代码：

### 1. 下行转码与 WebRTC 推流模块 (`internal/voicecall/webrtc.go`)

```go
// startOutgoingAudioStream 启动下行音频流: 模组声卡 PCM ➔ ffmpeg 编码为 Opus ➔ Pion WebRTC Track ➔ 浏览器
func (gw *WebRTCGateway) startOutgoingAudioStream(ctx context.Context) {
	if gw.audioTrack == nil {
		return
	}

	// 1. 下行 FFmpeg 转码器: 从 AudioBridge 读取 8kHz S16_LE PCM ➔ 编码为 Opus (48kHz 单声道，低延迟 voip 模式)
	gw.downEncoder = exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", "8000", "-ac", "1", "-i", "pipe:0",
		"-c:a", "libopus",
		"-ar", "48000",
		"-ac", "1",
		"-b:a", "24000",
		"-application", "voip",
		"-frame_duration", "20",
		"-page_duration", "20000",
		"-flush_packets", "1",
		"-f", "opus",
		"pipe:1",
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

	// 立即写入 3 帧静音 PCM 预热，确保 ffmpeg 瞬间输出 Ogg Header 与首包
	silenceWarmup := make([]byte, 320*3)
	_, _ = downIn.Write(silenceWarmup)

	// 启动后台协程: 从 AudioBridge 读取 PCM 并喂给 ffmpeg 编码器
	go func() {
		pcmBuf := make([]byte, 320) // 8kHz 16-bit 20ms = 320 bytes
		silence := make([]byte, 320)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := gw.audioBridge.ReadPCM(pcmBuf)
				if err != nil || n == 0 {
					if ctx.Err() != nil {
						return
					}
					// 声卡暂未产出数据时，持续输入静音包保活，维持 WebRTC 时间戳平滑
					_, _ = downIn.Write(silence)
					time.Sleep(20 * time.Millisecond)
					continue
				}
				if n > 0 {
					if gw.recorder != nil {
						gw.recorder.PushDownstream(pcmBuf[:n])
					}
					_, writeErr := downIn.Write(pcmBuf[:n])
					if writeErr != nil {
						if ctx.Err() != nil {
							return
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		}
	}()

	// 启动 OggReader 循环: 解析 FFmpeg 输出的 Opus 数据包并推入 WebRTC Track
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Warn("下行音频推流协程 panic 恢复", "recover", r)
			}
		}()

		var ogg *oggreader.OggReader
		var hdr *oggreader.OggHeader
		var err error

		// 重试读取 Ogg 头信息，等待 FFmpeg 吐出头部
		for retry := 0; retry < 10; retry++ {
			if ctx.Err() != nil {
				return
			}
			ogg, hdr, err = oggreader.NewWith(gw.downOpusOut)
			if err == nil && ogg != nil && hdr != nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if err != nil || ogg == nil {
			logger.Warn("解析下行 Opus Ogg 流头部失败", "err", err)
			return
		}

		// 持续读取 Ogg Page 并写入 WebRTC LocalTrack
		for {
			select {
			case <-ctx.Done():
				return
			default:
				pageData, pageHdr, pageErr := ogg.ParseNextPage()
				if pageErr != nil {
					if ctx.Err() != nil {
						return
					}
					time.Sleep(10 * time.Millisecond)
					continue
				}
				if len(pageData) == 0 {
					continue
				}
				sampleDuration := 20 * time.Millisecond
				if pageHdr != nil && pageHdr.GranulePosition > 0 {
					sampleDuration = 20 * time.Millisecond
				}
				writeErr := gw.audioTrack.WriteSample(media.Sample{
					Data:     pageData,
					Duration: sampleDuration,
				})
				if writeErr != nil {
					if ctx.Err() != nil {
						return
					}
				}
			}
		}
	}()
}
```

### 2. 声卡音频管道桥接 (`internal/voicecall/audio_bridge.go`)

```go
type AudioBridge struct {
	mu          sync.Mutex
	cardName    string
	arecordCmd  *exec.Cmd
	aplayCmd    *exec.Cmd
	sink        io.WriteCloser // 上行写入 aplay
	source      io.Reader      // 下行读取 arecord
	sourceClose io.Closer
	cancel      context.CancelFunc
	isOpen      bool
}

func (ab *AudioBridge) Open(ctx context.Context) error {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.isOpen && ab.source != nil && ab.sink != nil {
		return nil
	}

	card := ab.cardName
	if card == "" {
		card = "hw:0,0"
	}

	cCtx, cancel := context.WithCancel(context.Background())
	ab.cancel = cancel

	// 启动 arecord 采集下行声音 (8kHz 16bit Mono)
	arecord := exec.CommandContext(cCtx, "arecord",
		"-D", card,
		"-f", "S16_LE",
		"-r", "8000",
		"-c", "1",
		"-t", "raw",
		"-q",
		"-N",
		"--buffer-size=1024",
	)
	recOut, err := arecord.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := arecord.Start(); err != nil {
		cancel()
		return err
	}
	ab.arecordCmd = arecord
	ab.source = recOut
	ab.sourceClose = recOut

	// 启动 aplay 播放上行声音
	aplay := exec.CommandContext(cCtx, "aplay",
		"-D", card,
		"-f", "S16_LE",
		"-r", "8000",
		"-c", "1",
		"-t", "raw",
		"-q",
		"-N",
		"--buffer-size=1024",
	)
	playIn, err := aplay.StdinPipe()
	if err != nil {
		cancel()
		_ = arecord.Process.Kill()
		return err
	}
	if err := aplay.Start(); err != nil {
		cancel()
		_ = arecord.Process.Kill()
		return err
	}
	ab.aplayCmd = aplay
	ab.sink = playIn
	ab.isOpen = true

	return nil
}

func (ab *AudioBridge) ReadPCM(buf []byte) (int, error) {
	ab.mu.Lock()
	src := ab.source
	isOpen := ab.isOpen
	ab.mu.Unlock()

	if !isOpen || src == nil {
		return 0, io.EOF
	}
	return src.Read(buf)
}
```

### 3. 前端 WebRTC 管理器 (`web/src/composables/useVoiceCall.ts`)

```typescript
// 建立 WebRTC 对等连接
async function startWebRTC() {
  cleanupWebRTC()
  pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
  })

  // 监听远端音频 Track (下行声音)
  pc.ontrack = (event) => {
    if (event.track.kind === 'audio') {
      remoteStream = event.streams[0] || new MediaStream([event.track])
      if (remoteAudioEl) {
        remoteAudioEl.srcObject = remoteStream
        remoteAudioEl.play().catch((err) => {
          console.warn('远端音频自动播放受限:', err)
        })
      }
    }
  }

  // 采集本地麦克风 (上行声音)
  const localStream = await navigator.mediaDevices.getUserMedia({
    audio: {
      echoCancellation: true,
      noiseSuppression: true,
      autoGainControl: true,
      sampleRate: 48000
    }
  })
  localStream.getAudioTracks().forEach((track) => {
    pc?.addTrack(track, localStream)
  })

  // 创建 Offer 并通过 HTTP POST 提交给服务端
  const offer = await pc.createOffer({ offerToReceiveAudio: true })
  await pc.setLocalDescription(offer)
  const answer = await api.post('/api/voice/webrtc/offer', { sdp: offer.sdp })
  await pc.setRemoteDescription({ type: 'answer', sdp: answer.sdp })
}
```

---

## 🔍 关键排查方向与引导问题

请你针对以上代码和架构，从以下几个深度技术维度进行逐一排查并指出漏洞：

1. **FFmpeg Ogg/Opus 输出与 Pion `oggreader` 的协议边界问题**：
   - FFmpeg 使用 `-f opus pipe:1` 输出流时，其内部封装的 Ogg Page 结构与 Pion 的 `oggreader.ParseNextPage()` 是否存在不兼容？
   - 当 FFmpeg 采用 `-flush_packets 1` 和 `-page_duration 20000` 时，一个 Ogg Page 里面到底包含了多少个 Opus 帧？`oggreader.ParseNextPage()` 返回的 `pageData` 直接当作单个 `media.Sample` 塞给 WebRTC Track 是否符合 RTP Opus 封装规范（RFC 7587）？
   - 为什么 `oggreader.NewWith(gw.downOpusOut)` 在重试读取时可能会失败或阻塞？

2. **ALSA `arecord` 驱动层与标准流读取阻塞**：
   - `arecord -t raw -D hw:0,0` 默认输出的数据流，在 Go 的 `recOut.Read(pcmBuf)` 中是否存在严重缓冲或阻塞？
   - 当 `AudioBridge.Open()` 被并发调用或多次调用时，`arecord` 是否真正处于运行采集中？如果模组物理声卡在未接通时没有音频时钟输出，`arecord` 会发生什么？

3. **并发时序、管道生命周期与优雅退出**：
   - 每次通话挂断（`hangupLocked`）到下次呼叫开始，`startOutgoingAudioStream` 中的多个后台 goroutine 是否存在**泄露、竞争或死锁**？
   - `downIn.Write(silence)` 与 `AudioBridge.ReadPCM` 之间在没有互斥保护的情况下，FFmpeg 的 stdin 是否会被破坏？

4. **WebRTC SDP 协商与媒体流方向**：
   - 服务端在处理 WebRTC Offer 时生成 Answer，生成的 SDP 中音频媒体方向（`sendrecv` / `recvonly` / `sendonly`）是否与实际 Track 状态严格一致？
   - 浏览器是否因为接收到没有持续序列号/时间戳的 RTP 包而静默丢弃了音频？

---

## 💡 输出要求

1. **根因定位清单**：清晰列出导致“对方能听到我方，但我方听不到对方”的全部根本原因与潜在 Bug（按严重等级排序）。
2. **重构/修复方案**：
   - 提供彻底重构后的 `internal/voicecall/webrtc.go` 下行音频流水线实现代码；
   - 给出对于 `AudioBridge` 声卡读取与 `useVoiceCall.ts` 前端播放的必要改进代码；
   - 说明为什么新方案能彻底消除死锁、保证 100% 稳定的全双工通话下行音频。
