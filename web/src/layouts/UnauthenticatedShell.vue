<script setup lang="ts">
import { ref } from 'vue'
import LoadingScreen from '../components/LoadingScreen.vue'
import SwitchDark from '../components/SwitchDark.vue'
import UserGuideDialog from '../components/UserGuideDialog.vue'
import { QuestionCircle24Regular } from '@vicons/fluent'

defineProps({
  isDark: {
    type: Boolean,
    required: true
  }
})

const emit = defineEmits(['toggle-theme'])
const guideOpen = ref(false)
</script>

<template>
  <div class="h-screen flex items-center justify-center bg-gray-100 dark:bg-gray-950 transition-colors duration-300">
    <div class="absolute top-4 right-4 z-50 flex items-center gap-2">
      <el-tooltip content="使用指南与操作手册" placement="bottom" :show-after="200">
        <button
          type="button"
          @click="guideOpen = true"
          aria-label="使用指南与操作手册"
          class="w-8 h-8 rounded-full flex items-center justify-center transition-all duration-200 text-gray-600 hover:text-gray-900 bg-gray-100 hover:bg-gray-200 dark:text-gray-300 dark:hover:text-white dark:bg-white/10 dark:hover:bg-white/15 border border-gray-200/80 dark:border-white/10 focus:outline-none active:scale-95 cursor-pointer shadow-sm"
        >
          <el-icon :size="17"><QuestionCircle24Regular /></el-icon>
        </button>
      </el-tooltip>
      <SwitchDark :is-dark="isDark" @toggle="(e) => emit('toggle-theme', e)" />
    </div>
    <router-view v-slot="{ Component }">
      <Suspense>
        <template #default>
          <component :is="Component" />
        </template>
        <template #fallback>
          <LoadingScreen />
        </template>
      </Suspense>
    </router-view>

    <UserGuideDialog v-model="guideOpen" />
  </div>
</template>

