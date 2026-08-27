import { ref, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElNotification } from 'element-plus'

export interface CallSession {
  id: string
  device_id: string
  remote_number: string
  direction: 'inbound' | 'outbound'
  state: 'idle' | 'dialing' | 'ringing' | 'active' | 'terminated'
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
      // 幂等保护：如果已经在处理挂断（resetTimer 已设），或 localStream 已经释放，不重复处理
      if (!localStream.value && !peerConnection.value) {
        // 已经清理过了，只需确保 session 状态正确
        if (resetTimer === null) {
          resetTimer = setTimeout(() => {
            if (currentSession.value && currentSession.value.state === 'terminated') {
              currentSession.value = null
            }
            resetTimer = null
          }, 1500)
        }
        return
      }

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

      // 1.5 秒后自动清空通话会话，恢复待机拨号键盘
      clearTimeout(resetTimer)
      resetTimer = setTimeout(() => {
        if (currentSession.value && currentSession.value.state === 'terminated') {
          currentSession.value = null
        }
        resetTimer = null
      }, 1500)
    } else if (event.type === 'connected' || (event.type === 'state_change' && event.session?.state === 'active')) {
      // 呼叫已接通
      startWebRTC()
    }
  }

  // 启动 WebRTC 双向音频流
  async function startWebRTC() {
    ensureAudioElement()
    if (peerConnection.value && peerConnection.value.connectionState === 'connected') {
      return
    }

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

      // 1. 获取麦克风音频
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true
        },
        video: false
      })
      localStream.value = stream

      // 2. 创建 PeerConnection
      const pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
      })
      peerConnection.value = pc

      // 添加本地麦克风音轨
      stream.getTracks().forEach((track) => pc.addTrack(track, stream))

      // 接收远端模组声音
      pc.ontrack = (event) => {
        if (audioElement.value && event.streams[0]) {
          audioElement.value.srcObject = event.streams[0]
          audioElement.value.play().catch((e) => console.log('Audio autoplay blocked', e))
        }
      }

      // 监听连接状态：服务端挂断或异常断开时强制释放麦克风
      pc.onconnectionstatechange = () => {
        const state = pc.connectionState
        if (state === 'failed' || state === 'closed' || state === 'disconnected') {
          console.log('PeerConnection 状态变为', state, '，强制释放麦克风')
          releaseMedia()
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
      isConnecting.value = false
      console.warn('WebRTC 启动状态:', err.message || err)
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
        currentSession.value = null
      }, 1000)
    } catch (err: any) {
      ElMessage.error(err.message || '挂断失败')
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
    if (!localStream.value) return
    isMuted.value = !isMuted.value
    localStream.value.getAudioTracks().forEach((track) => {
      track.enabled = !isMuted.value
    })
    ElMessage.info(isMuted.value ? '已静音麦克风' : '已取消静音')
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
    releaseMedia()
  })

  return {
    currentSession,
    isInCall,
    isIncoming,
    isMuted,
    isConnecting,
    dial,
    answer,
    hangup,
    sendDTMF,
    toggleMute,
    connectWebSocket
  }
}
