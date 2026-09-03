import { ref, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElNotification } from 'element-plus'

export interface CallSession {
  id: string
  device_id: string
  remote_number: string
  direction: 'inbound' | 'outbound'
  state: 'idle' | 'dialing' | 'ringing' | 'active' | 'terminated'
  is_recording?: boolean
  started_at?: string
  connected_at?: string
  ended_at?: string
  duration_sec: number
  hangup_reason?: string
}

export interface CallEvent {
  type: string
  session?: CallSession
  timestamp: number
  message?: string
}

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

// 标准 ITU-T 双音多频 (DTMF) 频率表 (Hz)
const DTMF_FREQS: Record<string, [number, number]> = {
  '1': [697, 1209],
  '2': [697, 1336],
  '3': [697, 1477],
  '4': [770, 1209],
  '5': [770, 1336],
  '6': [770, 1477],
  '7': [852, 1209],
  '8': [852, 1336],
  '9': [852, 1477],
  '*': [941, 1209],
  '0': [941, 1336],
  '#': [941, 1477]
}

let dtmfAudioCtx: AudioContext | null = null
const isKeyToneEnabled = ref<boolean>(typeof window !== 'undefined' ? localStorage.getItem('ecall_key_tone') !== 'false' : true)

function toggleKeyTone() {
  isKeyToneEnabled.value = !isKeyToneEnabled.value
  if (typeof window !== 'undefined') {
    localStorage.setItem('ecall_key_tone', String(isKeyToneEnabled.value))
  }
}

/**
 * 播放逼真的手机拨号标准 DTMF 按键音
 * 纯本地 Web Audio API 独立输出，与 WebRTC 媒体流完全隔离，绝不影响通话
 */
function playDTMFTone(digit: string, durationMs: number = 100) {
  if (!isKeyToneEnabled.value || typeof window === 'undefined') return
  const freqs = DTMF_FREQS[digit]
  if (!freqs) return

  try {
    const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext
    if (!AudioContextClass) return

    if (!dtmfAudioCtx) {
      dtmfAudioCtx = new AudioContextClass()
    } else if (dtmfAudioCtx.state === 'suspended') {
      dtmfAudioCtx.resume().catch(() => {})
    }

    const now = dtmfAudioCtx.currentTime
    const duration = durationMs / 1000

    // 主增益与指数淡出（防爆音）
    const masterGain = dtmfAudioCtx.createGain()
    masterGain.gain.setValueAtTime(0.12, now)
    masterGain.gain.exponentialRampToValueAtTime(0.001, now + duration)
    masterGain.connect(dtmfAudioCtx.destination)

    // 低频正弦波
    const osc1 = dtmfAudioCtx.createOscillator()
    osc1.type = 'sine'
    osc1.frequency.setValueAtTime(freqs[0], now)
    osc1.connect(masterGain)
    osc1.start(now)
    osc1.stop(now + duration)

    // 高频正弦波
    const osc2 = dtmfAudioCtx.createOscillator()
    osc2.type = 'sine'
    osc2.frequency.setValueAtTime(freqs[1], now)
    osc2.connect(masterGain)
    osc2.start(now)
    osc2.stop(now + duration)
  } catch (e) {
    console.debug('播放 DTMF 失败', e)
  }
}

export function useVoiceCall() {
  const isInCall = computed(() => {
    return currentSession.value && ['dialing', 'ringing', 'active'].includes(currentSession.value.state)
  })

  const isIncoming = computed(() => {
    return currentSession.value && currentSession.value.direction === 'inbound' && currentSession.value.state === 'ringing'
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

  // 硬件媒体流硬释放守卫函数
  function releaseMedia() {
    if (localStream.value) {
      localStream.value.getTracks().forEach((track) => {
        try {
          track.stop()
        } catch (e) {
          console.warn('停止音频 track 失败', e)
        }
      })
      localStream.value = null
    }
    if (peerConnection.value) {
      try {
        peerConnection.value.close()
      } catch (e) {
        console.warn('关闭 PeerConnection 失败', e)
      }
      peerConnection.value = null
    }
    if (audioElement.value) {
      audioElement.value.srcObject = null
      try {
        audioElement.value.pause()
      } catch {}
    }
    isConnecting.value = false
    isWebRTCStarting = false
    isMuted.value = false
  }

  // 初始化 WebSocket 监听
  function connectWebSocket() {
    if (typeof window === 'undefined') return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    const token = localStorage.getItem('token') || ''
    const wsUrl = `${protocol}//${host}/api/voice/ws?token=${encodeURIComponent(token)}`

    try {
      ws = new WebSocket(wsUrl)
      ws.onopen = () => {
        clearInterval(heartbeatTimer)
        heartbeatTimer = setInterval(() => {
          if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'ping' }))
          }
        }, 20000)
      }

      ws.onmessage = (ev) => {
        try {
          const event: CallEvent = JSON.parse(ev.data)
          handleCallEvent(event)
        } catch (e) {
          console.error('解析呼叫事件失败', e)
        }
      }

      ws.onclose = () => {
        clearInterval(heartbeatTimer)
        setTimeout(connectWebSocket, 3000)
      }
    } catch (e) {
      console.error('连接呼叫 WebSocket 失败', e)
    }
  }

  function handleCallEvent(event: CallEvent) {
    if (event.session) {
      currentSession.value = event.session
    }

    if (event.type === 'incoming' && event.session) {
      ElNotification({
        title: '4G 蜂窝来电',
        message: `来电号码: ${event.session.remote_number || '未知号码'}`,
        type: 'warning',
        duration: 8000,
        position: 'top-right'
      })
    } else if (event.type === 'hangup' || (event.session && event.session.state === 'terminated')) {
      releaseMedia()
      let tip = '通话已结束'
      if (event.session?.hangup_reason === 'remote_canceled') {
        tip = '对方已取消呼叫'
      } else if (event.session?.hangup_reason === 'busy') {
        tip = '对方拒接或占线'
      } else if (event.session?.hangup_reason === 'no_answer') {
        tip = '对方无人接听'
      } else if (event.session?.hangup_reason === 'dial_failed') {
        tip = '拨号失败'
      }
      ElMessage.info(tip)

      // 1 秒后自动清空通话会话，恢复待机拨号键盘 (防止早退导致状态残留)
      clearTimeout(resetTimer)
      resetTimer = setTimeout(() => {
        currentSession.value = null
        resetTimer = null
      }, 1000)
    } else if (event.type === 'connected') {
      // 呼叫首次接通 (仅由 CLCC 接通事件触发一次)
      startWebRTC()
    } else if (event.type === 'recording_change' && event.session) {
      // 录音状态变更事件：仅同步录音状态，坚决不重新协商 WebRTC
      if (currentSession.value) {
        currentSession.value.is_recording = event.session.is_recording
      }
    }
  }

  // 启动 WebRTC 双向音频流
  async function startWebRTC() {
    ensureAudioElement()
    // 防重入互斥锁：只要正在建立或已存在有效 PeerConnection，坚决不重复创建
    if (isWebRTCStarting) {
      return
    }
    if (peerConnection.value && !['failed', 'closed'].includes(peerConnection.value.connectionState)) {
      return
    }
    isWebRTCStarting = true

    try {
      isConnecting.value = true

      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        const isHttp = window.location.protocol === 'http:' && !['localhost', '127.0.0.1'].includes(window.location.hostname)
        if (isHttp) {
          const { ElMessageBox } = await import('element-plus')
          ElMessageBox.alert(
            `由于现代浏览器安全策略，HTTP 局域网地址默认禁止调用麦克风。<br><br>
            <b>解决方法（任选一种）：</b><br>
            1. <b>使用 HTTPS 域名访问</b>（已配置）：直接访问 <code>https://ecall.omguy.top:57575</code> 即可原生支持麦克风；<br>
            2. <b>Chrome / Edge 白名单</b>：打开 <code>chrome://flags/#unsafely-treat-insecure-origin-as-secure</code>，添加 <code>${window.location.origin}</code> 并设为 Enabled。`,
            '麦克风权限受限 (HTTP 安全策略)',
            {
              dangerouslyUseHTMLString: true,
              confirmButtonText: '我知道了',
              type: 'warning'
            }
          ).catch(() => {})
          throw new Error('当前非安全上下文 (HTTP)，麦克风接口被浏览器禁用')
        }
        throw new Error('当前浏览器不支持 mediaDevices.getUserMedia')
      }

      // 1. 获取麦克风音频 (禁用 AGC 自动增益，防止说话时底噪被剧烈放大产生跟随抽吸电流声)
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: false,
          channelCount: 1
        },
        video: false
      })
      localStream.value = stream
      // 如果当前处于静音状态，立即应用到新音频流
      if (isMuted.value) {
        stream.getAudioTracks().forEach((track) => {
          track.enabled = false
        })
      }

      // 2. 创建 PeerConnection (配置国内低延迟 STUN 服务器)
      const pc = new RTCPeerConnection({
        iceServers: [
          { urls: 'stun:stun.qq.com:3478' },
          { urls: 'stun:stun.miwifi.com:3478' }
        ]
      })
      peerConnection.value = pc

      // 添加本地麦克风音轨
      stream.getTracks().forEach((track) => pc.addTrack(track, stream))

      // 接收远端模组声音
      pc.ontrack = (event) => {
        ensureAudioElement()
        if (audioElement.value) {
          const stream = (event.streams && event.streams[0]) ? event.streams[0] : new MediaStream([event.track])
          audioElement.value.srcObject = stream
          audioElement.value.play().catch((e) => console.log('Audio autoplay blocked', e))
        }
      }

      // 监听连接状态：
      pc.onconnectionstatechange = () => {
        // 关键防护 1：如果是旧连接的异步回调，坚决忽略，绝不能误杀当前新连接！
        if (pc !== peerConnection.value) return

        const state = pc.connectionState
        console.log('PeerConnection 状态变更:', state)
        // 关键防护 2：切勿在 'disconnected' 时立即销毁！网络短暂抖动会自动重连，只有 'failed' 才是真正的终态异常
        if (state === 'failed') {
          console.warn('PeerConnection 彻底失败，释放媒体资源')
          releaseMedia()
          // 关键修复：连接彻底失败时强制复位当前会话，坚决防止拨号盘锁死
          clearTimeout(resetTimer)
          resetTimer = setTimeout(() => {
            currentSession.value = null
            resetTimer = null
          }, 500)
          ElMessage.error('语音媒体流连接失败，已自动复位')
        }
      }

      // 收集 ICE 候选并同步
      pc.onicecandidate = (event) => {
        if (event.candidate) {
          fetch('/api/voice/webrtc/candidate', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${localStorage.getItem('token') || ''}`
            },
            body: JSON.stringify({
              candidate: event.candidate.candidate,
              sdpMid: event.candidate.sdpMid,
              sdpMLineIndex: event.candidate.sdpMLineIndex
            })
          }).catch((err) => console.error('发送 ICE Candidate 失败', err))
        }
      }

      // 创建 Offer
      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)

      // 发送 Offer 给后端
      const res = await fetch('/api/voice/webrtc/offer', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        },
        body: JSON.stringify({ sdp: offer.sdp })
      })

      if (!res.ok) {
        throw new Error('WebRTC 协商失败')
      }

      const answerData = await res.json()
      await pc.setRemoteDescription(new RTCSessionDescription({ type: 'answer', sdp: answerData.sdp }))
      isConnecting.value = false
    } catch (err: any) {
      console.warn('WebRTC 启动状态:', err.message || err)
    } finally {
      isConnecting.value = false
      isWebRTCStarting = false
    }
  }

  function stopWebRTC() {
    releaseMedia()
  }

  // 拨号
  async function dial(number: string, deviceId = '') {
    if (!number) {
      ElMessage.warning('请输入要拨打的电话号码')
      return
    }
    clearTimeout(resetTimer)
    ensureAudioElement()

    try {
      const res = await fetch('/api/voice/dial', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        },
        body: JSON.stringify({ number, device_id: deviceId })
      })

      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || '拨号失败')
      }

      const session: CallSession = await res.json()
      currentSession.value = session
      ElMessage.success(`正在呼叫 ${number}...`)
      // 拨号瞬间立即启动 WebRTC，听取回铃音/彩铃
      startWebRTC()
    } catch (err: any) {
      ElMessage.error(err.message || '呼叫失败')
    }
  }

  // 接听
  async function answer() {
    ensureAudioElement()
    try {
      const res = await fetch('/api/voice/answer', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        }
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || '接听失败')
      }
      const session: CallSession = await res.json()
      currentSession.value = session
      startWebRTC()
      ElMessage.success('已接听')
    } catch (err: any) {
      ElMessage.error(err.message || '接听失败')
    }
  }

  // 挂断
  async function hangup() {
    try {
      await fetch('/api/voice/hangup', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        }
      })
      releaseMedia()
      if (currentSession.value) {
        currentSession.value.state = 'terminated'
      }
      clearTimeout(resetTimer)
      resetTimer = setTimeout(() => {
        if (currentSession.value?.state === 'terminated') {
          currentSession.value = null
        }
      }, 1000)
    } catch (err: any) {
      ElMessage.error(err.message || '挂断失败')
    }
  }

  // 手动开启/停止录音
  async function toggleRecording() {
    if (!currentSession.value || currentSession.value.state !== 'active') {
      ElMessage.warning('仅在通话接通后支持录音')
      return
    }

    const isRec = !!currentSession.value.is_recording
    const action = isRec ? 'stop' : 'start'
    try {
      const res = await fetch(`/api/voice/recording/${action}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        }
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || (isRec ? '停止录音失败' : '开启录音失败'))
      }
      if (isRec) {
        currentSession.value.is_recording = false
        ElMessage.success('已停止录音并保存')
      } else {
        currentSession.value.is_recording = true
        ElMessage.success('已开始通话双向录音')
      }
    } catch (e: any) {
      ElMessage.error(e.message || '操作录音失败')
    }
  }

  // 发送 DTMF 按键
  async function sendDTMF(digit: string) {
    try {
      await fetch('/api/voice/dtmf', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`
        },
        body: JSON.stringify({ digit })
      })
    } catch (e) {
      console.error('发送 DTMF 失败', e)
    }
  }

  // 切换静音
  function toggleMute() {
    if (!localStream.value) {
      ElMessage.warning('麦克风音频流尚未就绪')
      return
    }
    isMuted.value = !isMuted.value
    // 1. 同步本地 MediaStream 轨道的启用状态
    localStream.value.getAudioTracks().forEach((track) => {
      track.enabled = !isMuted.value
    })
    // 2. 双重保障：同步 WebRTC PeerConnection 发送端 (RTCRtpSender) 上的轨道状态
    if (peerConnection.value) {
      peerConnection.value.getSenders().forEach((sender) => {
        if (sender.track && sender.track.kind === 'audio') {
          sender.track.enabled = !isMuted.value
        }
      })
    }
    ElMessage.info(isMuted.value ? '已静音麦克风' : '已取消静音，麦克风已恢复')
  }

  onMounted(() => {
    connectWebSocket()
    // 启动时长计时器
    clearInterval(durationTimer)
    durationTimer = setInterval(() => {
      if (currentSession.value && currentSession.value.state === 'active') {
        if (currentSession.value.connected_at) {
          const start = new Date(currentSession.value.connected_at).getTime()
          currentSession.value.duration_sec = Math.max(0, Math.floor((Date.now() - start) / 1000))
        } else if (currentSession.value.started_at) {
          const start = new Date(currentSession.value.started_at).getTime()
          currentSession.value.duration_sec = Math.max(0, Math.floor((Date.now() - start) / 1000))
        }
      }
    }, 1000)
  })

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
    isKeyToneEnabled,
    toggleKeyTone,
    playDTMFTone,
    dial,
    answer,
    hangup,
    toggleRecording,
    sendDTMF,
    toggleMute,
    connectWebSocket
  }
}
