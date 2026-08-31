# VoHive 语音通话重大异常排查 Prompt（关键黄金线索：录音文件有声，但实时通话双方无声）

---

## 一、系统架构与业务背景

VoHive 是一个基于 **4G/5G 蜂窝模组（Quectel / 大疆 QDC507 硬件平台，Linux Docker 环境）** 的 VoLTE WebRTC 语音网关：
1. **硬件与系统层**：
   - 蜂窝模组通过 USB 暴露 AT 串口控制（`/dev/ttyUSB*`）与 USB UAC PCM 物理声卡（`hw:0,0`，标准 8000Hz 16-bit Mono S16_LE）；
2. **音频下行链路（对方声音 / 运营商彩铃提示音 $\to$ 浏览器）**：
   `模组声卡 hw:0,0` $\to$ `arecord 8k Mono` $\to$ `AudioBridge.ReadPCM()` $\to$ `PCM Channel (双协程解耦缓冲)` $\to$ `20ms Ticker 恒定时钟` $\to$ `FFmpeg (PCM 8k -> Ogg/Opus 48k)` $\to$ `oggreader` $\to$ `Pion WebRTC Track` $\to$ `浏览器 <audio>`；
3. **音频上行链路（浏览器麦克风 $\to$ 对方）**：
   `浏览器麦克风` $\to$ `Pion WebRTC OnTrack` $\to$ `oggwriter` $\to$ `FFmpeg (Opus -> PCM 8k Mono)` $\to$ `AudioBridge.WritePCM()` $\to$ `aplay 8k Mono` $\to$ `模组声卡 hw:0,0`；
4. **服务端双向混音录音引擎（CallRecorder）**：
   在 `webrtc.go` 中，下行 PCM 流动时调用 `PushDownstream(chunk)`，上行 FFmpeg Opus 解码出 PCM 后调用 `PushUpstream(pcmBuf[:n])`，由录音引擎内部按 20ms 对齐混音喂给 `ffmpeg -c:a libmp3lame` 生成 MP3。

---

## 二、刚刚完成的第一阶段后端重构（解决推流死锁）

此前根据诊断，完成了后端下行链路与 ALSA 的重构：
1. **下行 PCM 采集与 FFmpeg 喂入拆为双协程**：
   - 协程 A 专职阻塞从声卡 `ReadPCM`，放入 `pcmChan := make(chan []byte, 50)`；
   - 协程 B 专职由 20ms Ticker 驱动从 `pcmChan` 取数据（无数据补静音包），持续向 FFmpeg `downIn` 写入，启动注入 10 帧静音预热，移除 `oggreader` 破坏性重试；
2. **ALSA 进程生命周期加固**：
   - `AudioBridge.Close()` 与 `Open()` 清理旧进程时，使用 `stopCmdSync()` 同步等待子进程退出释放声卡句柄。

---

## 三、重大实测转折点与关键黄金线索（极其重要）

### 黄金线索 1：未更新前端录音按键前，通话与声音完全正常
* 在完成第一阶段后端重构后、**前端 UI 尚未更新录音按键之前**：
  * 实测两次被叫来电接听：**双方均能清晰听到对方，双向语音完全正常**；
  * 实测呼出被对方拒接：**我方浏览器清晰听到运营商的拒接提示音**（证明早媒体与下行通道正常）。

### 黄金线索 2：更新前端录音 UI 后，实时通话双方无声，但服务端录音文件却有完整声音！
* **现象描述**：
  * 在前端（`useVoiceCall.ts` 与 `Voice.vue`）增加了手动录音按键与生命周期调整后，测试呼叫：
  * **实时通话双方听不到对方声音，呼出被拒接也听不到运营商提示音**；
  * 但是，**在双方都听不到声音的情况下点击录音，通话结束后去听生成的 MP3 录音文件，发现录音文件里双方的声音全部都清晰录下来了！**

---

## 四、黄金线索带来的物理推论与范围锁定

「**实时通话双方无声，但服务端录音文件里双方声音完整清晰**」这一事实，严格证明了以下结论：
1. **模组硬件声卡（ALSA `arecord` / `aplay` / `hw:0,0`）与基带硬件 100% 正常**：
   - 如果模组声卡未激活或物理层静音，服务端录音引擎绝不可能录到对方声音；
2. **服务端采集协程（协程 A `ReadPCM`）与上行 WebRTC 麦克风接收 100% 正常**：
   - `PushDownstream` 成功拿到声卡 PCM，`PushUpstream` 成功从浏览器 WebRTC 接收并解码出麦克风 PCM；
3. **故障被 100% 精确收敛到两个边界**：
   - **下行问题（我方听不到对方）**：PCM 进入了下行 FFmpeg 编码器，但为什么浏览器 `<audio>` 没有播放出声音？（是 FFmpeg Opus 转码推给 WebRTC Track 失败？还是前端 WebRTC `pc.ontrack` / `<audio>` 播放器绑定状态/自动播放受阻？）
   - **上行问题（对方听不到我方）**：上行 PCM 已经解码成功（录音中有声音），但调用 `gw.audioBridge.WritePCM(pcmBuf)` 写入 `aplay` 时，为什么对方听不到？（是 `aplay` 写入阻塞/丢包？还是基带在某种状态下没有把 UAC 音频混合进 VoLTE？）

---

## 五、前后端关键改动代码对比

### 1. 前端 `web/src/composables/useVoiceCall.ts`（本次新增录音按键时的改动）

```typescript
// 模块级单例响应式状态
const currentSession = ref<CallSession | null>(null)
const isMuted = ref(false)
const isConnecting = ref(false)
const localStream = shallowRef<MediaStream | null>(null)
const peerConnection = shallowRef<RTCPeerConnection | null>(null)
const audioElement = shallowRef<HTMLAudioElement | null>(null)

let ws: WebSocket | null = null
let heartbeatTimer: any = null
let durationTimer: any = null
let resetTimer: any = null
let isWebRTCStarting = false

export function useVoiceCall() {
  const isInCall = computed(() => {
    return currentSession.value && ['dialing', 'ringing', 'active'].includes(currentSession.value.state)
  })

  // 初始化隐藏的远端播放器
  function ensureAudioElement() {
    if (!audioElement.value && typeof document !== 'undefined') {
      const el = document.createElement('audio')
      el.autoplay = true
      el.style.display = 'none'
      document.body.appendChild(el)
      audioElement.value = el
    }
  }

  // 接收远端声音 Track
  pc.ontrack = (event) => {
    ensureAudioElement()
    if (audioElement.value) {
      const stream = (event.streams && event.streams[0]) ? event.streams[0] : new MediaStream([event.track])
      audioElement.value.srcObject = stream
      audioElement.value.play().catch((e) => console.log('Audio autoplay blocked', e))
    }
  }

  // 拨号入口
  async function dial(number: string, deviceId?: string) {
    clearTimeout(resetTimer)
    ensureAudioElement()
    ...
    startWebRTC()
  }

  // 挂断
  async function hangup() {
    ...
    releaseMedia()
    clearTimeout(resetTimer)
    resetTimer = setTimeout(() => {
      if (currentSession.value?.state === 'terminated') {
        currentSession.value = null
      }
    }, 1000)
  }

  // 本次新增的录音切换方法
  async function toggleRecording() {
    if (!currentSession.value || currentSession.value.state !== 'active') {
      ElMessage.warning('仅在通话接通后支持录音')
      return
    }
    const isRec = !!currentSession.value.is_recording
    const action = isRec ? 'stop' : 'start'
    const res = await fetch(`/api/voice/recording/${action}`, { method: 'POST', ... })
    if (isRec) {
      currentSession.value.is_recording = false
    } else {
      currentSession.value.is_recording = true
    }
  }

  onUnmounted(() => {
    // 活跃通话中切换页面不销毁媒体连接
    if (!isInCall.value) {
      releaseMedia()
    }
  })

  return {
    currentSession,
    isInCall,
    isIncoming,
    isMuted,
    isConnecting,
    isRecording: computed(() => !!currentSession.value?.is_recording),
    dial,
    answer,
    hangup,
    toggleRecording,
    sendDTMF,
    toggleMute
  }
}
```

### 2. 前端 `web/src/views/Voice.vue`（在通话界面中解构与放置录音按钮）

```vue
<script setup lang="ts">
const {
  currentSession,
  isInCall,
  isMuted,
  isConnecting,
  isRecording,
  dial,
  hangup,
  toggleRecording,
  sendDTMF,
  toggleMute
} = useVoiceCall()
</script>

<template>
  ...
  <!-- In-Call Active View -->
  <div v-if="isInCall && currentSession" class="py-6 text-center space-y-6">
    ...
    <div class="flex items-center justify-center gap-5 pt-4">
      <!-- 静音按键 -->
      <button @click.stop="toggleMute">...</button>

      <!-- 手动录音控制按键 -->
      <button
        type="button"
        :disabled="currentSession.state !== 'active'"
        @click.stop="toggleRecording"
        :class="[
          'flex h-14 w-14 items-center justify-center rounded-full transition-all shadow-md',
          isRecording ? 'bg-rose-600 text-white animate-pulse' : 'bg-gray-100 text-gray-700'
        ]"
      >
        <el-icon :size="24">
          <RecordStop24Filled v-if="isRecording" />
          <Record24Regular v-else />
        </el-icon>
      </button>

      <!-- DTMF 键盘按键 -->
      <button @click="showDTMF = !showDTMF">...</button>

      <!-- 挂断按键 -->
      <button @click="hangup">...</button>
    </div>
  </div>
</template>
```

### 3. 服务端 `internal/voicecall/webrtc.go`（下行与上行推流转码）

```go
// 协程 A: 采集声卡 PCM
go func() {
    pcmBuf := make([]byte, 320)
    for {
        select {
        case <-ctx.Done():
            return
        default:
            n, err := gw.audioBridge.ReadPCM(pcmBuf)
            if err != nil {
                time.Sleep(20 * time.Millisecond)
                continue
            }
            if n > 0 {
                data := make([]byte, n)
                copy(data, pcmBuf[:n])
                select {
                case pcmChan <- data:
                case <-ctx.Done():
                    return
                default:
                }
            }
        }
    }
}()

// 协程 B: 20ms 恒定时钟喂入 FFmpeg
go func() {
    ticker := time.NewTicker(20 * time.Millisecond)
    defer ticker.Stop()
    silence := make([]byte, 320)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            var chunk []byte
            select {
            case chunk = <-pcmChan:
            default:
                chunk = silence
            }

            // 录音引擎从这里拿到下行音频（实测录音文件里有声音！）
            if gw.recorder != nil && len(chunk) > 0 {
                gw.recorder.PushDownstream(chunk)
            }

            // 写入下行 FFmpeg 编码器
            _, _ = downIn.Write(chunk)
        }
    }
}()

// WebRTC 推流协程: 从 FFmpeg downOpusOut 读取 Ogg/Opus 封包写入 WebRTC audioTrack
func (gw *WebRTCGateway) startOutgoingAudioStream(ctx context.Context) {
    defer func() {
        if gw.downOpusOut != nil {
            _ = gw.downOpusOut.Close()
        }
    }()

    // 一次性初始化 oggreader
    ogg, _, err := oggreader.NewWith(gw.downOpusOut)
    if err != nil {
        logger.Warn("初始化 oggreader 失败", "err", err)
        return
    }

    for {
        select {
        case <-ctx.Done():
            return
        default:
            pageData, _, err := ogg.ParseNextPage()
            if err != nil {
                if err == io.EOF || ctx.Err() != nil {
                    return
                }
                time.Sleep(10 * time.Millisecond)
                continue
            }

            sample := media.Sample{
                Data:     pageData,
                Duration: 20 * time.Millisecond,
            }
            if err := gw.audioTrack.WriteSample(sample); err != nil {
                if ctx.Err() != nil {
                    return
                }
            }
        }
    }
}
```

---

## 六、请重点分析的疑点与诊断目标

结合**「录音文件有完整声音，但双方实时通话无声；且在前端增加录音按键前实时通话正常」**这一核心事实，请重点分析：

1. **下行无声（我方听不到对方）的根本原因定位**：
   - 既然 `PushDownstream` 拿到了声卡数据，说明声卡和协程 A/B 正常。那么数据在从 `downIn.Write(chunk)` $\to$ `gw.downEncoder` $\to$ `downOpusOut` $\to$ `oggreader.ParseNextPage()` $\to$ `gw.audioTrack.WriteSample(sample)` $\to$ 浏览器 WebRTC $\to$ `<audio>` 的链条中，断在了哪里？
   - 启动时的 10 帧静音预热（`320*10` 字节）在 `oggreader.NewWith` 处理后，后续的 Opus 数据流是否因为时间戳/时钟问题或 FFmpeg 转码滤镜（`aresample/dynaudnorm`）被截断或丢包？
   - 前端 Vue 组件在 `In-Call` 重新渲染或多次实例化 `useVoiceCall()` 时，`<audio>` 元素是否存在被重复创建、挂载失效、没有触发 `play()` 或媒体流音轨未启用的问题？
2. **上行无声（对方听不到我方）的根本原因定位**：
   - 既然录音文件里能录到我方麦克风声音（说明浏览器麦克风 $\to$ WebRTC $\to$ 上行 FFmpeg Opus 解码为 PCM 完全成功），那为什么 `gw.audioBridge.WritePCM` 写入 `aplay` 后，对方却听不到？
   - 是 `aplay` 进程写入死锁、缓存区堆积，还是模组基带在连续呼叫切换时内部音频通道静音？
3. **前端 `useVoiceCall` 模块单例与 Vue 视图绑定的潜在隐患**：
   - `audioElement` / `peerConnection` / `localStream` 作为全局响应式变量，在视图重构引入录音按钮后是否存在生命周期或闭包覆盖问题？
4. **请给出最精准的根因结论与完整的修复代码方案。**
