<script setup lang="ts">
import { WeatherMoon24Regular, WeatherSunny24Regular } from '@vicons/fluent'

const props = defineProps<{
  isDark: boolean
}>()

const emit = defineEmits<{
  toggle: [next: boolean]
}>()

function onToggle() {
  emit('toggle', !props.isDark)
}
</script>

<template>
  <el-tooltip :content="isDark ? '切换为亮色模式' : '切换为暗色模式'" placement="bottom" :show-after="200">
    <button
      type="button"
      @click="onToggle"
      :aria-label="isDark ? '切换为亮色模式' : '切换为暗色模式'"
      class="theme-toggle-btn w-8 h-8 rounded-full flex items-center justify-center transition-all duration-200 text-gray-600 hover:text-gray-900 bg-gray-100 hover:bg-gray-200 dark:text-gray-300 dark:hover:text-white dark:bg-white/10 dark:hover:bg-white/15 border border-gray-200/80 dark:border-white/10 focus:outline-none active:scale-95 cursor-pointer shadow-sm"
    >
      <el-icon :size="17" class="theme-icon">
        <WeatherSunny24Regular v-if="isDark" />
        <WeatherMoon24Regular v-else />
      </el-icon>
    </button>
  </el-tooltip>
</template>

<style scoped>
.theme-toggle-btn {
  user-select: none;
}

.theme-icon {
  transition: transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.theme-toggle-btn:hover .theme-icon {
  transform: rotate(18deg) scale(1.08);
}
</style>
