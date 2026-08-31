<script setup lang="ts">
import type { DeviceOverviewItem } from '../types/api'
import { ArrowSync24Regular, Power24Regular, Mail24Regular, Call24Regular } from '@vicons/fluent'

defineProps<{
  device: DeviceOverviewItem
  rotating: boolean
  rebooting: boolean
  reconnectingVoWiFi: boolean
}>()

const emit = defineEmits<{
  'copy-text': [value: string]
  'rotate-ip': []
  'reboot-modem': []
  'reconnect-vowifi': []
  'open-sms': []
  'open-voice': []
}>()
</script>

<template>
  <div class="ui-card p-6">
    <div class="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
      <div class="min-w-0">
        <div class="flex items-center gap-3">
          <div class="device-header-brand-icon">
            <svg class="w-5 h-5 text-white" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M4 6.5C4 5.12 5.12 4 6.5 4H7.8C8.5 4 9.1 4.5 9.25 5.2L9.9 8.2C10 8.7 9.8 9.3 9.4 9.6L8.1 10.7C9.2 13.1 10.9 14.8 13.3 15.9L14.4 14.6C14.7 14.2 15.3 14 15.8 14.1L18.8 14.75C19.5 14.9 20 15.5 20 16.2V17.5C20 18.88 18.88 20 17.5 20C10.04 20 4 13.96 4 6.5Z" fill="currentColor"/>
              <path d="M14 4C17.31 4 20 6.69 20 10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
              <path d="M14 7.5C15.93 7.5 17.5 9.07 17.5 11" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
            </svg>
          </div>
          <div class="min-w-0">
            <div class="text-xl font-extrabold text-gray-900 dark:text-white truncate">{{ device.name || device.id }}</div>
            <div class="text-xs text-gray-500 dark:text-gray-400 mt-0.5 truncate">
              <span class="font-mono cursor-pointer hover:underline" @click="emit('copy-text', device.id)">{{ device.id }}</span>
              · 公网 IP:
              <span class="font-mono cursor-pointer hover:underline" @click="emit('copy-text', device.public_ip || '')">{{ device.public_ip || '---' }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <el-button v-if="device?.vowifi_enabled" :loading="reconnectingVoWiFi" @click="emit('reconnect-vowifi')" class="ui-glass-border !border-0">
          <el-icon><ArrowSync24Regular /></el-icon>
          重连 VoWiFi
        </el-button>
        <el-button v-else :loading="rotating" :disabled="!device?.network_connected" @click="emit('rotate-ip')" class="ui-glass-border !border-0">
          <el-icon><ArrowSync24Regular /></el-icon>
          切换 IP
        </el-button>
        <el-button :loading="rebooting" @click="emit('reboot-modem')" class="ui-glass-border !border-0 hover:!text-red-600">
          <el-icon><Power24Regular /></el-icon>
          重启模组
        </el-button>
        <el-button @click="emit('open-sms')" class="ui-glass-border !border-0">
          <el-icon><Mail24Regular /></el-icon>
          短信
        </el-button>
        <el-button @click="emit('open-voice')" class="ui-glass-border !border-0 hover:!text-emerald-600">
          <el-icon><Call24Regular /></el-icon>
          电话
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.device-header-brand-icon {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: linear-gradient(135deg, #5b5bd6, #4a4ac2);
  color: #fff;
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 1.15rem;
  font-weight: 700;
  box-shadow: 0 10px 22px rgba(91, 91, 214, 0.2);
}

</style>
