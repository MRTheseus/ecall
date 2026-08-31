<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { ElMessage } from 'element-plus'
import PageHeader from '../components/PageHeader.vue'
import { ArrowDownload24Regular, Delete24Regular, Pause24Regular, Play24Regular } from '@vicons/fluent'
import { useLogsStore } from '../stores/logs'
import { useEventStream } from '../composables/useEventStream'

// 日志条目类型
interface LogEntry {
  time: string
  level: string
  caller: string
  message: string
  fields?: string
}

const logsStore = useLogsStore()
const { logs } = storeToRefs(logsStore)

const connected = ref(false)
const paused = ref(false)
const autoScroll = ref(true)
const levelFilter = ref<'all' | 'debug' | 'info' | 'warn' | 'error'>('all')
const searchQuery = ref('')
const maxLogs = 1000 // 最大保留日志条数
const lastConnectError = ref<string>('')

// 日志容器引用
const logContainer = ref<HTMLElement | null>(null)

// 过滤后的日志
const filteredLogs = computed(() => {
  let result = logs.value

  // 级别过滤（精确匹配选中的级别）
  if (levelFilter.value !== 'all') {
    result = result.filter(log => 
      log.level.toLowerCase() === levelFilter.value.toLowerCase()
    )
  }

  // 搜索过滤
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(log =>
      log.message.toLowerCase().includes(q) ||
      log.caller.toLowerCase().includes(q) ||
      (log.fields && log.fields.toLowerCase().includes(q))
    )
  }

  return result
})

const stream = useEventStream<LogEntry>({
  path: '/logs/stream',
  eventName: 'log',
  query: { level: '' },
  parse: (payload) => JSON.parse(payload) as LogEntry,
  onConnected: () => {
    connected.value = true
    lastConnectError.value = ''
  },
  onEvent: (entry) => {
    if (paused.value) return
    logsStore.append(entry, maxLogs)
    if (!autoScroll.value) return
    nextTick(() => {
      if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
    })
  }
})

function connect() {
  connected.value = false
  stream.setPaused(false)
}

function disconnect() {
  stream.disconnect()
  connected.value = false
}

// 暂停/继续
function togglePause() {
  paused.value = !paused.value
  stream.setPaused(paused.value)
  if (!paused.value) connect()
}

// 清空日志
function clearLogs() {
  logsStore.clear()
}

// 导出日志
function exportLogs() {
  const content = filteredLogs.value.map(log => {
    const time = new Date(log.time).toLocaleString()
    const fields = log.fields ? ` ${log.fields}` : ''
    return `[${time}] ${log.level.toUpperCase().padEnd(5)} ${log.caller} ${log.message}${fields}`
  }).join('\n')

  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs-${new Date().toISOString().slice(0, 10)}.txt`
  a.click()
  URL.revokeObjectURL(url)
  ElMessage.success('已导出日志')
}

// 日志级别颜色
function getLevelClass(level: string): string {
  switch (level.toLowerCase()) {
    case 'debug': return 'text-purple-500'
    case 'info': return 'text-blue-500'
    case 'warn': return 'text-yellow-500'
    case 'error': return 'text-red-500'
    case 'fatal': return 'text-red-600 font-bold'
    default: return 'text-gray-500'
  }
}

// 格式化日期时间
function formatDateTime(isoTime: string): string {
  try {
    const d = new Date(isoTime)
    const yyyy = d.getFullYear()
    const MM = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    const HH = String(d.getHours()).padStart(2, '0')
    const mm = String(d.getMinutes()).padStart(2, '0')
    const ss = String(d.getSeconds()).padStart(2, '0')
    return `${yyyy}-${MM}-${dd} ${HH}:${mm}:${ss}`
  } catch {
    return isoTime
  }
}

// 加载历史日志
async function loadHistory() {
  const result = await logsStore.fetchHistory(500)
  if (!result.ok) return
  nextTick(() => {
    if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
  })
}

onMounted(async () => {
  await loadHistory()
  connect()
})

onUnmounted(() => {
  disconnect()
})

watch(levelFilter, () => {
  stream.setQuery({ level: levelFilter.value === 'all' ? '' : levelFilter.value })
  if (!paused.value) {
    connect()
  }
})
</script>

<template>
  <div class="space-y-4">
    <!-- 控制栏与状态指示 -->
    <div class="ui-card p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
      <div class="flex items-center gap-3 flex-wrap">
        <div class="flex items-center gap-2">
          <span class="w-2.5 h-2.5 rounded-full" :class="connected ? 'bg-emerald-500 animate-pulse' : 'bg-rose-500'" />
          <span class="text-xs font-semibold" :class="connected ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-500'">
            {{ connected ? '已连接实时日志流' : '未连接' }}
          </span>
        </div>
        <span class="text-xs text-gray-400 font-mono">({{ filteredLogs.length }} / {{ logs.length }} 条)</span>
        <span v-if="!connected && lastConnectError" class="text-xs text-rose-500 truncate" :title="lastConnectError">{{ lastConnectError }}</span>
      </div>

      <div class="flex items-center gap-2 flex-wrap">
        <el-checkbox v-model="autoScroll" label="自动滚屏" size="small" class="!mr-2" />
        <el-button @click="togglePause" :type="paused ? 'success' : 'warning'" size="small" class="!border-0">
          <el-icon><component :is="paused ? Play24Regular : Pause24Regular" /></el-icon>
          {{ paused ? '继续' : '暂停' }}
        </el-button>
        <el-button @click="clearLogs" size="small" class="!border-0">
          <el-icon><Delete24Regular /></el-icon>
          清空
        </el-button>
        <el-button @click="exportLogs" type="primary" size="small" class="!border-0">
          <el-icon><ArrowDownload24Regular /></el-icon>
          导出
        </el-button>
      </div>
    </div>

    <!-- 过滤器与搜索 -->
    <div class="ui-card p-4">
      <div class="flex flex-wrap items-center gap-3">
        <el-select v-model="levelFilter" placeholder="日志级别" class="w-28" size="default">
          <el-option label="全部级别" value="all" />
          <el-option label="DEBUG" value="debug" />
          <el-option label="INFO" value="info" />
          <el-option label="WARN" value="warn" />
          <el-option label="ERROR" value="error" />
        </el-select>
        <el-input
          v-model="searchQuery"
          placeholder="搜索日志关键词 (支持模块/函数/消息)..."
          clearable
          class="flex-1 min-w-[200px]"
          size="default"
        />
      </div>
    </div>

    <!-- 日志流控制台视窗 -->
    <div class="ui-card overflow-hidden rounded-2xl border border-gray-200 dark:border-gray-800 shadow-md">
      <div
        ref="logContainer"
        class="h-[62vh] overflow-auto font-mono text-xs leading-relaxed bg-gray-950 text-gray-100 p-4 select-text"
      >
        <div v-if="filteredLogs.length === 0" class="text-gray-500 text-center py-16">
          {{ connected ? '暂无匹配日志输出...' : '正在连接实时日志服务...' }}
        </div>
        <div
          v-for="(log, idx) in filteredLogs"
          :key="idx"
          class="py-1 hover:bg-white/5 px-2 -mx-2 rounded flex items-start gap-2 border-b border-white/[0.03]"
        >
          <span class="text-gray-500 shrink-0 font-mono">[{{ formatDateTime(log.time) }}]</span>
          <span class="font-bold shrink-0 w-12" :class="getLevelClass(log.level)">{{ log.level.toUpperCase().padEnd(5) }}</span>
          <span class="text-indigo-400 shrink-0 max-w-[180px] truncate" :title="log.caller">{{ log.caller }}</span>
          <span class="text-gray-200 break-all flex-1">{{ log.message }}</span>
          <span v-if="log.fields" class="text-amber-300/80 break-all font-mono text-[11px]">{{ log.fields }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
