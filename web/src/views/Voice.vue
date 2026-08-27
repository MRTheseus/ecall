<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useVoiceCall } from '../composables/useVoiceCall'
import { api } from '../stores/auth'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Call24Filled,
  CallEnd24Filled,
  Mic24Regular,
  MicOff24Filled,
  Backspace24Regular,
  NumberSymbol24Regular,
  Dialpad24Regular,
  Star24Regular,
  History24Regular,
  CallMissed24Regular,
  ArrowUpRight24Regular,
  ArrowDownLeft24Regular,
  Delete24Regular,
  Mail24Regular
} from '@vicons/fluent'
import { Loading } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const queryDevice = typeof route.query.device === 'string' ? route.query.device : ''
const dialNumber = ref('')
const selectedDeviceId = ref(queryDevice || localStorage.getItem('ecall_voice_device') || '')
const showDTMF = ref(false)

function goToSMS(phone: string) {
  const query: Record<string, string> = { phone }
  if (selectedDeviceId.value) {
    query.device = selectedDeviceId.value
  }
  router.push({ path: '/sms', query })
}

const availableDevices = ref<any[]>([])
const topContacts = ref<{ remote_number: string; call_count: number; last_call_at: string }[]>([])
const callRecords = ref<any[]>([])
const loadingRecords = ref(false)

const {
  currentSession,
  isInCall,
  isMuted,
  isConnecting,
  dial,
  hangup,
  sendDTMF,
  toggleMute
} = useVoiceCall()

const dialpadKeys = [
  { digit: '1', sub: '' },
  { digit: '2', sub: 'ABC' },
  { digit: '3', sub: 'DEF' },
  { digit: '4', sub: 'GHI' },
  { digit: '5', sub: 'JKL' },
  { digit: '6', sub: 'MNO' },
  { digit: '7', sub: 'PQRS' },
  { digit: '8', sub: 'TUV' },
  { digit: '9', sub: 'WXYZ' },
  { digit: '*', sub: '' },
  { digit: '0', sub: '+' },
  { digit: '#', sub: '' }
]

function appendDigit(digit: string) {
  if (isInCall.value) {
    sendDTMF(digit)
  } else {
    dialNumber.value += digit
  }
}

function handleBackspace() {
  if (dialNumber.value.length > 0) {
    dialNumber.value = dialNumber.value.slice(0, -1)
  }
}

function handleCall() {
  if (!dialNumber.value) return
  dial(dialNumber.value, selectedDeviceId.value)
}

const selectedContactNumber = ref<string>('')

function selectContact(num: string) {
  if (selectedContactNumber.value === num) {
    fillNumber(num)
  } else {
    selectedContactNumber.value = num
  }
}

function fillNumber(num: string) {
  dialNumber.value = num
  selectedContactNumber.value = num
  ElMessage.success(`已填入号码：${num}`)
}

// 格式化通话时间 (秒 -> mm:ss)
const formattedDuration = computed(() => {
  const sec = currentSession.value?.duration_sec || 0
  const m = Math.floor(sec / 60).toString().padStart(2, '0')
  const s = (sec % 60).toString().padStart(2, '0')
  return `${m}:${s}`
})

const callStatusText = computed(() => {
  if (!currentSession.value) return ''
  switch (currentSession.value.state) {
    case 'dialing':
      return '正在呼叫...'
    case 'ringing':
      return currentSession.value.direction === 'inbound' ? '来电振铃中...' : '对方正在响铃...'
    case 'active':
      return '正在通话'
    case 'terminated':
      if (currentSession.value.hangup_reason === 'remote_canceled') return '对方已取消呼叫'
      if (currentSession.value.hangup_reason === 'busy') return '对方拒接/占线'
      if (currentSession.value.hangup_reason === 'no_answer') return '无人接听'
      return '通话已结束'
    default:
      return ''
  }
})

// 拉取设备列表
async function fetchDevices() {
  try {
    const res = await api.get('/devices')
    const list = res.data?.devices || []
    availableDevices.value = list
    
    // 如果 URL 参数指定了有效设备，优先选中
    const qDev = typeof route.query.device === 'string' ? route.query.device : ''
    if (qDev && list.some((d: any) => d.id === qDev)) {
      selectedDeviceId.value = qDev
      return
    }

    // 自动选中在线设备
    const running = list.filter((d: any) => d.running)
    if (running.length > 0) {
      if (!selectedDeviceId.value || !running.some((d: any) => d.id === selectedDeviceId.value)) {
        selectedDeviceId.value = running[0].id
      }
    } else if (list.length > 0 && !selectedDeviceId.value) {
      selectedDeviceId.value = list[0].id
    }
  } catch (e) {
    console.error('拉取设备列表失败', e)
  }
}

// 拉取最高频 Top 3 号码
async function fetchTopContacts() {
  try {
    const res = await api.get('/voice/top-contacts', { params: { limit: 3 } })
    topContacts.value = res.data?.contacts || []
  } catch (e) {
    console.error('获取高频联系人失败', e)
  }
}

// 拉取通话记录
async function fetchCallRecords() {
  loadingRecords.value = true
  try {
    const res = await api.get('/voice/records', { params: { limit: 50 } })
    callRecords.value = res.data?.records || []
  } catch (e) {
    console.error('获取通话记录失败', e)
  } finally {
    loadingRecords.value = false
  }
}

// 删除单条通话记录
async function handleDeleteRecord(id: number) {
  try {
    await api.delete(`/voice/records/${id}`)
    ElMessage.success('已删除记录')
    await fetchCallRecords()
    await fetchTopContacts()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '删除失败')
  }
}

// 清空所有通话记录
async function handleClearHistory() {
  try {
    await ElMessageBox.confirm('确定要清空所有通话记录吗？此操作不可恢复。', '清空记录', {
      confirmButtonText: '确定清空',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await api.delete('/voice/records')
    ElMessage.success('通话记录已全部清空')
    await fetchCallRecords()
    await fetchTopContacts()
  } catch (e) {
    // cancelled
  }
}

// 格式化通话记录时间
function formatCallTime(ts: string) {
  if (!ts) return ''
  const date = new Date(ts)
  if (isNaN(date.getTime())) return ''
  const now = new Date()
  const isToday = date.toDateString() === now.toDateString()
  const hours = date.getHours().toString().padStart(2, '0')
  const minutes = date.getMinutes().toString().padStart(2, '0')
  if (isToday) {
    return `${hours}:${minutes}`
  }
  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (date.toDateString() === yesterday.toDateString()) {
    return `昨天 ${hours}:${minutes}`
  }
  const month = (date.getMonth() + 1).toString().padStart(2, '0')
  const day = date.getDate().toString().padStart(2, '0')
  return `${month}/${day} ${hours}:${minutes}`
}

function formatDuration(sec: number) {
  if (!sec || sec <= 0) return '00:00'
  const m = Math.floor(sec / 60).toString().padStart(2, '0')
  const s = (sec % 60).toString().padStart(2, '0')
  return `${m}:${s}`
}

function formatCallStatus(item: any) {
  if (item.direction === 'inbound') {
    if (item.state === 'completed') return '来电接听'
    if (item.state === 'missed') return '未接来电'
    return '来电未接通'
  }
  if (item.state === 'completed') return '呼出'
  if (item.state === 'busy') return '对方拒接'
  return '未接通'
}

watch(selectedDeviceId, (val) => {
  if (val) {
    localStorage.setItem('ecall_voice_device', val)
  }
})

watch(() => route.query.device, (newDev) => {
  if (typeof newDev === 'string' && newDev) {
    selectedDeviceId.value = newDev
  }
})

// 监听通话结束，自动刷新通话记录与高频联系人
watch(() => currentSession.value?.state, (newState) => {
  if (newState === 'terminated') {
    setTimeout(() => {
      fetchCallRecords()
      fetchTopContacts()
    }, 600)
  }
})

onMounted(() => {
  fetchDevices()
  fetchTopContacts()
  fetchCallRecords()
})
</script>

<template>
  <div class="h-full w-full overflow-y-auto p-4 sm:p-6 lg:p-8">
    <div class="mx-auto max-w-5xl space-y-6">
      
      <!-- 页面头部与设备切换器 -->
      <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 bg-white dark:bg-gray-800/80 rounded-2xl px-5 py-3.5 shadow-sm border border-gray-100 dark:border-gray-700/60 backdrop-blur-xl">
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-bold text-gray-900 dark:text-white flex items-center gap-2">
            电话拨号
            <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-indigo-50 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
              VoLTE
            </span>
          </h1>
        </div>

        <!-- 呼叫设备选择卡片 -->
        <div class="flex items-center gap-2 flex-wrap">
          <span class="text-xs font-semibold text-gray-400 dark:text-gray-500 whitespace-nowrap">呼叫设备:</span>
          <div v-if="availableDevices.length === 0" class="text-xs text-gray-400">
            暂无可用设备
          </div>
          <button
            v-for="d in availableDevices"
            :key="d.id"
            type="button"
            :disabled="!!isInCall"
            @click="selectedDeviceId = d.id"
            class="flex items-center gap-2 px-3 py-1.5 rounded-xl border text-xs transition-all"
            :class="[
              selectedDeviceId === d.id
                ? 'border-indigo-500 bg-indigo-50/80 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 font-semibold shadow-xs ring-1 ring-indigo-500/20'
                : 'border-gray-200 dark:border-gray-700 bg-gray-50/80 dark:bg-gray-800/60 text-gray-600 dark:text-gray-400 hover:border-gray-300 dark:hover:border-gray-600 hover:text-gray-900 dark:hover:text-gray-200',
              isInCall ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'
            ]"
          >
            <span
              class="inline-block w-2 h-2 rounded-full shrink-0"
              :class="d.running ? 'bg-emerald-500' : 'bg-gray-400'"
              :title="d.running ? '在线' : '离线'"
            ></span>
            <span class="truncate max-w-[120px]">{{ d.name || d.id }}</span>
            <span v-if="d.name && d.name !== d.id" class="text-[10px] text-gray-400 font-mono">({{ d.id }})</span>
          </button>
        </div>
      </div>

      <!-- 拨号与通话主区域 -->
      <div class="grid grid-cols-1 md:grid-cols-12 gap-6 items-start">
        
        <!-- 左侧/主要：拨号面板 -->
        <div class="md:col-span-7 bg-white dark:bg-gray-800/80 rounded-3xl p-6 shadow-sm border border-gray-100 dark:border-gray-700/60 backdrop-blur-xl">
          
          <!-- 通话中视图 (In-Call Active View) -->
          <div v-if="isInCall && currentSession" class="py-6 text-center space-y-6">
            <div class="relative mx-auto flex h-28 w-28 items-center justify-center">
              <div class="absolute h-full w-full animate-ping rounded-full bg-emerald-500/20"></div>
              <div class="relative flex h-24 w-24 items-center justify-center rounded-full bg-gradient-to-tr from-emerald-500 to-teal-400 text-white shadow-xl">
                <el-icon :size="42"><Call24Filled /></el-icon>
              </div>
            </div>

            <div>
              <h2 class="text-3xl font-mono font-bold text-gray-900 dark:text-white">
                {{ currentSession.remote_number }}
              </h2>
              <p class="mt-2 text-sm font-medium text-emerald-600 dark:text-emerald-400">
                {{ callStatusText }} · {{ formattedDuration }}
              </p>
              <p v-if="isConnecting" class="mt-1 text-xs text-amber-500 animate-pulse">
                正在建立 WebRTC 麦克风音频流...
              </p>
            </div>

            <!-- 通话控制按钮组 -->
            <div class="flex items-center justify-center gap-6 pt-4">
              <!-- 静音麦克风 -->
              <button
                type="button"
                @click.stop="toggleMute"
                :class="[
                  'flex h-14 w-14 items-center justify-center rounded-full transition-all active:scale-95 shadow-md select-none',
                  isMuted
                    ? 'bg-amber-500 text-white ring-4 ring-amber-300/60 dark:ring-amber-600/60'
                    : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200'
                ]"
                :title="isMuted ? '点击恢复麦克风 (当前已静音)' : '静音麦克风'"
              >
                <el-icon :size="24">
                  <MicOff24Filled v-if="isMuted" />
                  <Mic24Regular v-else />
                </el-icon>
              </button>

              <!-- DTMF 键盘切换 -->
              <button
                @click="showDTMF = !showDTMF"
                :class="[
                  'flex h-14 w-14 items-center justify-center rounded-full transition-all active:scale-95 shadow-md',
                  showDTMF
                    ? 'bg-indigo-600 text-white'
                    : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200'
                ]"
                title="按键拨号"
              >
                <el-icon :size="24"><NumberSymbol24Regular /></el-icon>
              </button>

              <!-- 挂断 -->
              <button
                @click="hangup"
                class="flex h-16 w-16 items-center justify-center rounded-full bg-rose-500 text-white shadow-xl hover:bg-rose-600 transition-all hover:scale-105 active:scale-95"
                title="挂断"
              >
                <el-icon :size="32"><CallEnd24Filled /></el-icon>
              </button>
            </div>

            <!-- DTMF 数字键盘展开 -->
            <div v-if="showDTMF" class="pt-4 max-w-xs mx-auto">
              <p class="text-xs text-gray-400 mb-3">点击按键发送二次拨号音 (IVR 语音导航)</p>
              <div class="grid grid-cols-3 gap-3">
                <button
                  v-for="k in dialpadKeys"
                  :key="k.digit"
                  @click="appendDigit(k.digit)"
                  class="h-12 rounded-xl bg-gray-50 dark:bg-gray-700/60 text-lg font-bold font-mono hover:bg-indigo-500 hover:text-white transition-colors"
                >
                  {{ k.digit }}
                </button>
              </div>
            </div>
          </div>

          <!-- 空闲/拨号盘视图 (Dialpad View) -->
          <div v-else class="space-y-6">
            <!-- 号码显示输入框 -->
            <div class="relative flex items-center justify-center bg-gray-50 dark:bg-gray-900/50 rounded-2xl p-4 border border-gray-100 dark:border-gray-800">
              <input
                v-model="dialNumber"
                type="text"
                placeholder="输入电话号码..."
                @keyup.enter="handleCall"
                class="w-full text-center text-3xl font-mono font-bold tracking-wider bg-transparent text-gray-900 dark:text-white focus:outline-none placeholder:text-gray-300 dark:placeholder:text-gray-600 placeholder:text-xl"
              />
              <button
                v-if="dialNumber"
                @click="handleBackspace"
                class="absolute right-4 p-2 text-gray-400 hover:text-rose-500 transition-colors"
                title="退格"
              >
                <el-icon :size="24"><Backspace24Regular /></el-icon>
              </button>
            </div>

            <!-- 九宫格按键 -->
            <div class="grid grid-cols-3 gap-3.5 max-w-xs mx-auto">
              <button
                v-for="k in dialpadKeys"
                :key="k.digit"
                @click="appendDigit(k.digit)"
                class="group flex flex-col items-center justify-center h-16 rounded-2xl bg-gray-50/80 dark:bg-gray-700/40 hover:bg-indigo-500 hover:text-white active:scale-95 transition-all border border-transparent dark:border-gray-700/50 shadow-sm"
              >
                <span class="text-2xl font-bold font-mono group-hover:scale-110 transition-transform">{{ k.digit }}</span>
                <span v-if="k.sub" class="text-[10px] tracking-widest text-gray-400 group-hover:text-indigo-100 font-semibold">{{ k.sub }}</span>
              </button>
            </div>

            <!-- 呼叫大绿按钮 -->
            <div class="flex items-center justify-center gap-6 pt-2">
              <button
                @click="handleCall"
                :disabled="!dialNumber"
                class="flex h-16 w-36 items-center justify-center gap-2 rounded-full bg-gradient-to-r from-emerald-500 to-teal-500 text-white font-bold text-lg shadow-lg shadow-emerald-500/25 hover:opacity-95 active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed transition-all"
              >
                <el-icon :size="26"><Call24Filled /></el-icon>
                <span>呼叫</span>
              </button>
            </div>
          </div>
        </div>

        <!-- 右侧：快捷拨号 (Top 3) + 通话记录 -->
        <div class="md:col-span-5 space-y-6">
          
          <!-- 区域 A: 常用快捷呼叫 (动态 Top 3) -->
          <div class="bg-white dark:bg-gray-800/80 rounded-3xl p-5 shadow-sm border border-gray-100 dark:border-gray-700/60">
            <div class="flex items-center justify-between mb-3.5">
              <h3 class="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-1.5">
                <el-icon class="text-amber-500"><Star24Regular /></el-icon>
                常用快捷呼叫
              </h3>
              <span class="text-[11px] text-gray-400">近期高频</span>
            </div>

            <!-- 动态渲染 Top 3 联系人卡片 -->
            <div v-if="topContacts.length > 0" class="space-y-2.5">
              <div
                v-for="contact in topContacts"
                :key="contact.remote_number"
                @click="selectContact(contact.remote_number)"
                class="flex items-center justify-between p-3 rounded-2xl border transition-all cursor-pointer group select-none"
                :class="[
                  selectedContactNumber === contact.remote_number
                    ? 'border-indigo-400 dark:border-indigo-500/80 bg-indigo-50/80 dark:bg-indigo-950/40 ring-1 ring-indigo-500/20 shadow-xs'
                    : 'border-transparent hover:border-gray-200 dark:hover:border-gray-700 bg-gray-50/80 dark:bg-gray-700/30'
                ]"
              >
                <div class="flex items-center gap-3 min-w-0">
                  <div class="w-11 h-11 rounded-2xl bg-gradient-to-tr from-indigo-500 to-cyan-400 text-white font-bold flex items-center justify-center text-[11px] font-mono tracking-tight shadow-sm shrink-0">
                    {{ contact.remote_number.slice(-4) }}
                  </div>
                  <div class="min-w-0">
                    <div
                      class="font-bold font-mono text-sm text-gray-800 dark:text-gray-200 transition-colors truncate"
                      :class="{ 'text-indigo-600 dark:text-indigo-400 font-extrabold': selectedContactNumber === contact.remote_number }"
                    >
                      {{ contact.remote_number }}
                    </div>
                    <div class="text-[11px] text-gray-400 truncate">
                      累计通话 {{ contact.call_count }} 次
                    </div>
                  </div>
                </div>

                <!-- 操作按钮组：发短信 + 填入拨号盘 (选中时始终可见，PC端悬浮亦可见) -->
                <div
                  class="flex items-center gap-1.5 ml-2 shrink-0 transition-opacity"
                  :class="[
                    selectedContactNumber === contact.remote_number
                      ? 'opacity-100 pointer-events-auto'
                      : 'opacity-0 sm:group-hover:opacity-100 pointer-events-none sm:group-hover:pointer-events-auto'
                  ]"
                >
                  <button
                    type="button"
                    @click.stop="goToSMS(contact.remote_number)"
                    class="h-8 w-8 rounded-full bg-white dark:bg-gray-700 text-sky-600 dark:text-sky-400 hover:bg-sky-50 dark:hover:bg-sky-950/50 flex items-center justify-center shadow-xs border border-gray-200/60 dark:border-gray-600/50 transition-colors cursor-pointer"
                    title="发送短信"
                  >
                    <el-icon :size="15"><Mail24Regular /></el-icon>
                  </button>
                  <button
                    type="button"
                    @click.stop="fillNumber(contact.remote_number)"
                    class="h-8 w-8 rounded-full bg-emerald-500 hover:bg-emerald-600 text-white flex items-center justify-center shadow-xs transition-colors cursor-pointer"
                    title="填入拨号盘"
                  >
                    <el-icon :size="15"><Call24Filled /></el-icon>
                  </button>
                </div>
              </div>
            </div>

            <!-- 暂无通话记录时的占位引导 -->
            <div v-else class="text-center py-6 text-gray-400 dark:text-gray-500 text-xs">
              <p>暂无常用高频号码</p>
              <p class="text-[11px] text-gray-400/80 mt-1">拨打通话后将按频次自动统计在此</p>
            </div>
          </div>

          <!-- 区域 B: 手机端通话记录 (Recent Calls) -->
          <div class="bg-white dark:bg-gray-800/80 rounded-3xl p-5 shadow-sm border border-gray-100 dark:border-gray-700/60">
            <div class="flex items-center justify-between mb-3.5">
              <h3 class="text-sm font-bold text-gray-900 dark:text-white flex items-center gap-1.5">
                <el-icon class="text-indigo-500"><History24Regular /></el-icon>
                通话记录
              </h3>
              <button
                v-if="callRecords.length > 0"
                @click="handleClearHistory"
                class="text-[11px] text-gray-400 hover:text-rose-500 transition-colors cursor-pointer"
              >
                清空
              </button>
            </div>

            <!-- 记录列表 -->
            <div v-if="loadingRecords" class="py-10 text-center text-xs text-gray-400">
              <el-icon class="animate-spin mr-1"><Loading /></el-icon> 加载通话记录...
            </div>
            <div v-else-if="callRecords.length === 0" class="py-10 text-center text-xs text-gray-400">
              暂无历史通话记录
            </div>
            <div v-else class="space-y-1.5 max-h-[360px] overflow-y-auto pr-1">
              <div
                v-for="item in callRecords"
                :key="item.id"
                @click="fillNumber(item.remote_number)"
                class="flex items-center justify-between p-2.5 rounded-2xl hover:bg-gray-50 dark:hover:bg-gray-700/40 transition-all cursor-pointer group select-none"
              >
                <div class="flex items-center gap-3">
                  <!-- 呼入/呼出/未接状态图标 -->
                  <div
                    class="w-8 h-8 rounded-xl flex items-center justify-center text-xs flex-shrink-0"
                    :class="{
                      'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400': item.direction === 'inbound' && item.state === 'completed',
                      'bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-400': item.direction === 'outbound',
                      'bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-400': item.state === 'missed' || item.state === 'busy'
                    }"
                  >
                    <el-icon :size="16">
                      <ArrowDownLeft24Regular v-if="item.direction === 'inbound' && item.state === 'completed'" />
                      <ArrowUpRight24Regular v-else-if="item.direction === 'outbound'" />
                      <CallMissed24Regular v-else />
                    </el-icon>
                  </div>

                  <div class="min-w-0">
                    <div
                      class="font-mono font-semibold text-xs text-gray-900 dark:text-gray-100 truncate"
                      :class="{ 'text-rose-600 dark:text-rose-400 font-bold': item.state === 'missed' }"
                    >
                      {{ item.remote_number || '未知号码' }}
                    </div>
                    <div class="text-[10px] text-gray-400 flex items-center gap-1.5 mt-0.5">
                      <span>{{ formatCallStatus(item) }}</span>
                      <span v-if="item.duration_sec > 0">· {{ formatDuration(item.duration_sec) }}</span>
                    </div>
                  </div>
                </div>

                <div class="flex items-center gap-2">
                  <span class="text-[11px] text-gray-400 font-mono">
                    {{ formatCallTime(item.started_at) }}
                  </span>
                  <!-- 单条删除按钮 -->
                  <button
                    @click.stop="handleDeleteRecord(item.id)"
                    class="opacity-0 group-hover:opacity-100 text-gray-400 hover:text-rose-500 p-1 transition-opacity cursor-pointer"
                    title="删除此记录"
                  >
                    <el-icon :size="14"><Delete24Regular /></el-icon>
                  </button>
                </div>
              </div>
            </div>
          </div>

        </div>
      </div>
    </div>
  </div>
</template>
