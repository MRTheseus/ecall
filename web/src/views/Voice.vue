<script setup lang="ts">
import { ref, computed } from 'vue'
import { useVoiceCall } from '../composables/useVoiceCall'
import {
  Call24Filled,
  CallEnd24Filled,
  Mic24Regular,
  MicOff24Filled,
  Backspace24Regular,
  NumberSymbol24Regular,
  Dialpad24Regular
} from '@vicons/fluent'

const dialNumber = ref('')
const selectedDeviceId = ref('')
const showDTMF = ref(false)

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

function quickDial(num: string) {
  dialNumber.value = num
  dial(num, selectedDeviceId.value)
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
</script>

<template>
  <div class="h-full w-full overflow-y-auto p-4 sm:p-6 lg:p-8">
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- 页面头部 -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
            <el-icon class="text-indigo-500"><Dialpad24Regular /></el-icon>
            4G VoLTE 电话拨号
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            通过模组基带直接接打蜂窝网络电话 · 浏览器 WebRTC 实时双向麦克风与扬声器
          </p>
        </div>
      </div>

      <!-- 拨号与通话卡片区域 -->
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

        <!-- 右侧：快捷拨号与说明 -->
        <div class="md:col-span-5 space-y-6">
          <!-- 快捷服务热线 -->
          <div class="bg-white dark:bg-gray-800/80 rounded-3xl p-6 shadow-sm border border-gray-100 dark:border-gray-700/60">
            <h3 class="text-base font-bold text-gray-900 dark:text-white mb-4">
              常用快捷呼叫
            </h3>
            <div class="grid grid-cols-2 gap-3">
              <button
                @click="quickDial('10086')"
                class="flex items-center justify-between p-3 rounded-2xl bg-gray-50 dark:bg-gray-700/40 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 border border-transparent hover:border-emerald-200 dark:hover:border-emerald-800 transition-all text-left group"
              >
                <div>
                  <div class="font-bold font-mono text-gray-800 dark:text-gray-200">10086</div>
                  <div class="text-xs text-gray-400">中国移动客服</div>
                </div>
                <el-icon class="text-emerald-500 opacity-0 group-hover:opacity-100 transition-opacity"><Call24Filled /></el-icon>
              </button>

              <button
                @click="quickDial('10010')"
                class="flex items-center justify-between p-3 rounded-2xl bg-gray-50 dark:bg-gray-700/40 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 border border-transparent hover:border-emerald-200 dark:hover:border-emerald-800 transition-all text-left group"
              >
                <div>
                  <div class="font-bold font-mono text-gray-800 dark:text-gray-200">10010</div>
                  <div class="text-xs text-gray-400">中国联通客服</div>
                </div>
                <el-icon class="text-emerald-500 opacity-0 group-hover:opacity-100 transition-opacity"><Call24Filled /></el-icon>
              </button>

              <button
                @click="quickDial('10000')"
                class="flex items-center justify-between p-3 rounded-2xl bg-gray-50 dark:bg-gray-700/40 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 border border-transparent hover:border-emerald-200 dark:hover:border-emerald-800 transition-all text-left group"
              >
                <div>
                  <div class="font-bold font-mono text-gray-800 dark:text-gray-200">10000</div>
                  <div class="text-xs text-gray-400">中国电信客服</div>
                </div>
                <el-icon class="text-emerald-500 opacity-0 group-hover:opacity-100 transition-opacity"><Call24Filled /></el-icon>
              </button>

              <button
                @click="quickDial('10099')"
                class="flex items-center justify-between p-3 rounded-2xl bg-gray-50 dark:bg-gray-700/40 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 border border-transparent hover:border-emerald-200 dark:hover:border-emerald-800 transition-all text-left group"
              >
                <div>
                  <div class="font-bold font-mono text-gray-800 dark:text-gray-200">10099</div>
                  <div class="text-xs text-gray-400">中国广电客服</div>
                </div>
                <el-icon class="text-emerald-500 opacity-0 group-hover:opacity-100 transition-opacity"><Call24Filled /></el-icon>
              </button>
            </div>
          </div>

          <!-- 通话提示卡片 -->
          <div class="bg-gradient-to-br from-indigo-50 to-blue-50 dark:from-indigo-950/20 dark:to-blue-950/20 rounded-3xl p-6 border border-indigo-100/50 dark:border-indigo-900/30">
            <h4 class="font-bold text-indigo-900 dark:text-indigo-300 text-sm mb-2">使用提示</h4>
            <ul class="text-xs text-indigo-800/80 dark:text-indigo-300/80 space-y-2 list-disc list-inside leading-relaxed">
              <li><b>HTTP 局域网麦克风开启方式</b>：若使用 HTTP 局域网访问，请在 Chrome 地址栏打开 <code class="bg-indigo-100 dark:bg-indigo-900 px-1 py-0.5 rounded">chrome://flags/#unsafely-treat-insecure-origin-as-secure</code>（Edge 为 <code class="bg-indigo-100 dark:bg-indigo-900 px-1 py-0.5 rounded">edge://flags/...</code>），添加当前网址为安全源并启用，重启浏览器即可使用麦克风。</li>
              <li>通话建立后，系统将自动建立低延迟 WebRTC 媒体流通道。</li>
              <li>支持拨打手机、座机以及带 IVR 按键导航的服务号码。</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
