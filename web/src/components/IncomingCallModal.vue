<script setup lang="ts">
import { watch, onUnmounted } from 'vue'
import { useVoiceCall } from '../composables/useVoiceCall'
import { Call24Filled, Dismiss24Filled } from '@vicons/fluent'

const { currentSession, isIncoming, answer, hangup } = useVoiceCall()

let audioCtx: AudioContext | null = null
let ringTimer: any = null

function playRingTone() {
  stopRingTone()
  try {
    const AudioContextClass = window.AudioContext || (window as any).webkitAudioContext
    if (!AudioContextClass) return
    audioCtx = new AudioContextClass()

    const playBeep = () => {
      if (!audioCtx) return
      const osc1 = audioCtx.createOscillator()
      const osc2 = audioCtx.createOscillator()
      const gain = audioCtx.createGain()

      osc1.frequency.value = 440 // 440Hz
      osc2.frequency.value = 480 // 480Hz

      osc1.connect(gain)
      osc2.connect(gain)
      gain.connect(audioCtx.destination)

      gain.gain.setValueAtTime(0.2, audioCtx.currentTime)
      gain.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 1.2)

      osc1.start()
      osc2.start()
      osc1.stop(audioCtx.currentTime + 1.2)
      osc2.stop(audioCtx.currentTime + 1.2)
    }

    playBeep()
    ringTimer = setInterval(playBeep, 3000)
  } catch (e) {
    console.error('播放来电铃声失败', e)
  }
}

function stopRingTone() {
  if (ringTimer) {
    clearInterval(ringTimer)
    ringTimer = null
  }
  if (audioCtx) {
    audioCtx.close().catch(() => {})
    audioCtx = null
  }
}

watch(isIncoming, (incoming) => {
  if (incoming) {
    playRingTone()
  } else {
    stopRingTone()
  }
}, { immediate: true })

onUnmounted(() => {
  stopRingTone()
})

async function handleAnswer() {
  stopRingTone()
  await answer()
}

async function handleReject() {
  stopRingTone()
  await hangup()
}
</script>

<template>
  <Transition name="fade-scale">
    <div
      v-if="isIncoming && currentSession"
      class="fixed inset-0 z-[99999] flex items-center justify-center bg-black/60 backdrop-blur-md px-4"
    >
      <div class="relative w-full max-w-sm overflow-hidden rounded-3xl bg-white/95 p-8 text-center shadow-2xl backdrop-blur-xl dark:bg-gray-900/95 dark:text-white border border-white/20">
        <!-- 呼吸光环动画 -->
        <div class="relative mx-auto mb-6 flex h-24 w-24 items-center justify-center">
          <div class="absolute h-full w-full animate-ping rounded-full bg-emerald-500/30"></div>
          <div class="relative flex h-20 w-20 items-center justify-center rounded-full bg-gradient-to-tr from-emerald-500 to-teal-400 text-white shadow-lg">
            <el-icon :size="36" class="animate-bounce">
              <Call24Filled />
            </el-icon>
          </div>
        </div>

        <h3 class="text-xl font-bold text-gray-900 dark:text-gray-100">来电提醒</h3>
        <p class="mt-2 text-2xl font-mono font-semibold text-emerald-600 dark:text-emerald-400">
          {{ currentSession.remote_number || '未知号码' }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">4G 模组 VoLTE 呼入 · 正在振铃...</p>

        <!-- 操作按钮 -->
        <div class="mt-8 flex items-center justify-center gap-8">
          <!-- 拒接 -->
          <button
            @click="handleReject"
            class="group flex flex-col items-center gap-2 focus:outline-none"
          >
            <div class="flex h-16 w-16 items-center justify-center rounded-full bg-rose-500 text-white shadow-lg transition-transform hover:scale-110 active:scale-95">
              <el-icon :size="28"><Dismiss24Filled /></el-icon>
            </div>
            <span class="text-xs text-gray-600 dark:text-gray-300">拒接</span>
          </button>

          <!-- 接听 -->
          <button
            @click="handleAnswer"
            class="group flex flex-col items-center gap-2 focus:outline-none"
          >
            <div class="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-500 text-white shadow-lg transition-transform hover:scale-110 active:scale-95">
              <el-icon :size="28"><Call24Filled /></el-icon>
            </div>
            <span class="text-xs text-gray-600 dark:text-gray-300">接听</span>
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.fade-scale-enter-active,
.fade-scale-leave-active {
  transition: all 0.3s ease-out;
}
.fade-scale-enter-from,
.fade-scale-leave-to {
  opacity: 0;
  transform: scale(0.9);
}
</style>
