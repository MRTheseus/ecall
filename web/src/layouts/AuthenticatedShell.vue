<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { Expand, Fold } from '@element-plus/icons-vue'
import LoadingScreen from '../components/LoadingScreen.vue'
import ErrorBoundary from '../components/ErrorBoundary.vue'
import SwitchDark from '../components/SwitchDark.vue'
import UserGuideDialog from '../components/UserGuideDialog.vue'
import { debugCollector } from '../debug/collector'
import {
  Mail24Regular,
  Settings24Regular,
  SignOut24Regular,
  Board24Regular,
  Phone24Regular,
  Call24Regular,
  Globe24Regular,
  DocumentText24Regular,
  QuestionCircle24Regular
} from '@vicons/fluent'
import IncomingCallModal from '../components/IncomingCallModal.vue'
import { useSMSStore } from '../stores/sms'
import { useVoiceCall } from '../composables/useVoiceCall'

defineProps({
  isDark: {
    type: Boolean,
    required: true
  }
})

const emit = defineEmits(['toggle-theme'])

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const smsStore = useSMSStore()
const { unreadMissedCalls, fetchUnreadMissedCalls, markMissedCallsRead } = useVoiceCall()
const collapsed = ref(false)
const isMobile = ref(false)
const drawerOpen = ref(false)
const debugOpen = ref(false)
const guideOpen = ref(false)
const DebugPanel = defineAsyncComponent(() => import('../components/DebugPanel.vue'))

function getMenuBadge(path: string): number {
  if (path === '/sms') {
    return smsStore.totalUnreadCount || 0
  }
  if (path === '/voice') {
    return unreadMissedCalls.value || 0
  }
  return 0
}

let badgeTimer: any = null

function refreshBadges() {
  smsStore.fetchUnreadCount().catch(() => {})
  fetchUnreadMissedCalls().catch(() => {})
}

// 路由监听：进入电话拨号页面自动清除未接电话角标
watch(
  () => route.path,
  (path) => {
    if (path === '/voice') {
      markMissedCallsRead().catch(() => {})
    }
  },
  { immediate: true }
)

const menuItems = [
  { index: '/', label: '仪表盘', shortLabel: '仪表盘', icon: Board24Regular },
  { index: '/devices', label: '设备管理', shortLabel: '设备', icon: Phone24Regular },
  { index: '/voice', label: '电话拨号', shortLabel: '电话', icon: Call24Regular },
  { index: '/sms', label: '短信中心', shortLabel: '短信', icon: Mail24Regular },
  { index: '/settings', label: '系统设置', shortLabel: '设置', icon: Settings24Regular }
]

async function handleLogout() {
  const { ElMessageBox } = await import('element-plus')
  const confirmed = await ElMessageBox.confirm('确认退出登录？', '提示', {
    confirmButtonText: '退出',
    cancelButtonText: '取消',
    type: 'warning'
  })
    .then(() => true)
    .catch(() => false)
  if (!confirmed) return
  auth.logout()
  router.push('/login')
}

function syncIsMobile() {
  if (typeof window === 'undefined') return
  isMobile.value = window.matchMedia('(max-width: 767px)').matches
  if (!isMobile.value) {
    drawerOpen.value = false
  }
}

function handleNavToggle() {
  if (isMobile.value) {
    drawerOpen.value = true
  } else {
    collapsed.value = !collapsed.value
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && e.shiftKey && String(e.key || '').toLowerCase() === 'd') {
    e.preventDefault()
    debugOpen.value = !debugOpen.value
    localStorage.setItem('debug_panel_open', debugOpen.value ? '1' : '0')
  }
}

onMounted(() => {
  syncIsMobile()
  window.addEventListener('resize', syncIsMobile, { passive: true })

  const saved = localStorage.getItem('debug_panel_open')
  debugOpen.value = saved === '1'

  window.addEventListener('keydown', onKeydown)

  refreshBadges()
  badgeTimer = setInterval(refreshBadges, 15000)
})

onUnmounted(() => {
  window.removeEventListener('resize', syncIsMobile)
  window.removeEventListener('keydown', onKeydown)
  if (badgeTimer) {
    clearInterval(badgeTimer)
    badgeTimer = null
  }
})

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  }
)

watch(
  () => debugOpen.value,
  (v) => {
    localStorage.setItem('debug_panel_open', v ? '1' : '0')
  }
)

watch(
  () => debugCollector.openPanelRequestAt.value,
  (ts) => {
    if (!ts) return
    debugOpen.value = true
  }
)

const activePath = computed(() => route.path)
</script>

<template>
  <el-container v-if="auth.isAuthenticated && route.name !== 'Login'" class="h-full">
    <el-aside
      v-if="!isMobile"
      :width="collapsed ? '52px' : '232px'"
      class="h-full ui-glass transition-[width] duration-200 relative sidebar-shell"
    >
      <div class="h-14 px-4 flex items-center" :class="collapsed ? 'justify-center px-0' : ''">
        <div class="sidebar-brand-icon">
          <svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M4 6.5C4 5.12 5.12 4 6.5 4H7.8C8.5 4 9.1 4.5 9.25 5.2L9.9 8.2C10 8.7 9.8 9.3 9.4 9.6L8.1 10.7C9.2 13.1 10.9 14.8 13.3 15.9L14.4 14.6C14.7 14.2 15.3 14 15.8 14.1L18.8 14.75C19.5 14.9 20 15.5 20 16.2V17.5C20 18.88 18.88 20 17.5 20C10.04 20 4 13.96 4 6.5Z" fill="currentColor"/>
            <path d="M14 4C17.31 4 20 6.69 20 10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            <path d="M14 7.5C15.93 7.5 17.5 9.07 17.5 11" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
          </svg>
        </div>
        <div v-if="!collapsed" class="ml-3">
          <div class="sidebar-brand-title">Ecall</div>
        </div>
      </div>

      <el-menu
        :collapse="collapsed"
        :collapse-transition="false"
        :default-active="activePath"
        class="sidebar-menu !border-0 !border-r-0 !bg-transparent mt-2"
        router
      >
        <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index">
          <div class="relative flex items-center justify-center">
            <el-icon><component :is="item.icon" /></el-icon>
            <span
              v-if="collapsed && getMenuBadge(item.index) > 0"
              class="absolute -top-1 -right-2 min-w-[15px] h-3.5 px-1 rounded-full bg-rose-500 text-white text-[9px] font-bold flex items-center justify-center leading-none shadow-xs"
            >
              {{ getMenuBadge(item.index) > 99 ? '99+' : getMenuBadge(item.index) }}
            </span>
          </div>
          <template #title>
            <div class="w-full flex items-center justify-between pr-2">
              <span class="sidebar-menu-label">{{ item.label }}</span>
              <span
                v-if="getMenuBadge(item.index) > 0"
                class="px-1.5 py-0.5 text-[10px] font-bold rounded-full bg-rose-500 text-white leading-none shadow-xs"
              >
                {{ getMenuBadge(item.index) > 99 ? '99+' : getMenuBadge(item.index) }}
              </span>
            </div>
          </template>
        </el-menu-item>
      </el-menu>

      <div class="absolute bottom-4 w-full px-3" v-if="!collapsed">
        <div class="ui-panel-muted p-3 flex items-center gap-3">
          <div class="w-9 h-9 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-300">
            <el-icon><Settings24Regular /></el-icon>
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-sm font-bold truncate">Admin</div>
            <div class="text-xs text-gray-400 truncate">Administrator</div>
          </div>
          <el-button text type="danger" @click="handleLogout">
            <el-icon><SignOut24Regular /></el-icon>
          </el-button>
        </div>
      </div>
    </el-aside>

    <el-drawer v-model="drawerOpen" direction="ltr" size="256px" :with-header="false" class="mobile-drawer">
      <div class="h-full bg-white/95 dark:bg-[#141418]/95 backdrop-blur-md relative sidebar-shell">
        <div class="h-16 px-4 flex items-center">
          <div class="sidebar-brand-icon">
            <svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M4 6.5C4 5.12 5.12 4 6.5 4H7.8C8.5 4 9.1 4.5 9.25 5.2L9.9 8.2C10 8.7 9.8 9.3 9.4 9.6L8.1 10.7C9.2 13.1 10.9 14.8 13.3 15.9L14.4 14.6C14.7 14.2 15.3 14 15.8 14.1L18.8 14.75C19.5 14.9 20 15.5 20 16.2V17.5C20 18.88 18.88 20 17.5 20C10.04 20 4 13.96 4 6.5Z" fill="currentColor"/>
              <path d="M14 4C17.31 4 20 6.69 20 10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
              <path d="M14 7.5C15.93 7.5 17.5 9.07 17.5 11" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
            </svg>
          </div>
          <div class="ml-3">
            <div class="sidebar-brand-title">Ecall</div>
          </div>
        </div>

        <el-menu
          :collapse="false"
          :collapse-transition="false"
          :default-active="activePath"
          class="sidebar-menu !border-0 !border-r-0 !bg-transparent mt-2"
          router
        >
          <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index">
            <el-icon><component :is="item.icon" /></el-icon>
            <template #title>
              <div class="w-full flex items-center justify-between pr-2">
                <span class="sidebar-menu-label">{{ item.label }}</span>
                <span
                  v-if="getMenuBadge(item.index) > 0"
                  class="px-1.5 py-0.5 text-[10px] font-bold rounded-full bg-rose-500 text-white leading-none shadow-xs"
                >
                  {{ getMenuBadge(item.index) > 99 ? '99+' : getMenuBadge(item.index) }}
                </span>
              </div>
            </template>
          </el-menu-item>
        </el-menu>

        <div class="absolute bottom-4 w-full px-3">
          <div class="ui-panel-muted p-3 flex items-center gap-3">
            <div class="w-9 h-9 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-300">
              <el-icon><Settings24Regular /></el-icon>
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-bold truncate">Admin</div>
              <div class="text-xs text-gray-400 truncate">Administrator</div>
            </div>
            <el-button text type="danger" @click="handleLogout">
              <el-icon><SignOut24Regular /></el-icon>
            </el-button>
          </div>
        </div>
      </div>
    </el-drawer>

    <el-container class="h-full">
      <el-header class="h-14 px-4 sm:px-5 flex items-center justify-between ui-glass border-b border-gray-100 dark:border-white/5 sticky top-0 z-10">
        <div class="flex items-center gap-2.5">
          <el-button text @click="handleNavToggle" class="!px-2">
            <el-icon :size="18">
              <Fold v-if="!isMobile && !collapsed" />
              <Expand v-else />
            </el-icon>
          </el-button>

          <!-- 移动端顶部标题与图标 -->
          <div v-if="isMobile" class="flex items-center gap-2">
            <div class="sidebar-brand-icon !w-7 !h-7 !rounded-lg">
              <svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M4 6.5C4 5.12 5.12 4 6.5 4H7.8C8.5 4 9.1 4.5 9.25 5.2L9.9 8.2C10 8.7 9.8 9.3 9.4 9.6L8.1 10.7C9.2 13.1 10.9 14.8 13.3 15.9L14.4 14.6C14.7 14.2 15.3 14 15.8 14.1L18.8 14.75C19.5 14.9 20 15.5 20 16.2V17.5C20 18.88 18.88 20 17.5 20C10.04 20 4 13.96 4 6.5Z" fill="currentColor"/>
                <path d="M14 4C17.31 4 20 6.69 20 10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                <path d="M14 7.5C15.93 7.5 17.5 9.07 17.5 11" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
              </svg>
            </div>
            <span class="font-bold text-gray-800 dark:text-gray-200 text-sm tracking-tight">VoHive Ecall</span>
          </div>
        </div>

        <div class="flex items-center gap-2 sm:gap-2.5">
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

          <el-tooltip content="系统运行正常" placement="bottom" :show-after="300">
            <div class="hidden sm:flex items-center justify-center w-8 h-8 rounded-full bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-200/60 dark:border-emerald-500/20 shadow-xs">
              <span class="relative flex h-2 w-2">
                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
            </div>
          </el-tooltip>
        </div>
      </el-header>

      <el-main class="p-4 sm:p-6 overflow-auto bg-gray-50/50 dark:bg-transparent" :class="{ '!pb-24': isMobile }">
        <div class="main-inner mx-auto w-full">
          <router-view v-slot="{ Component, route: r }">
            <ErrorBoundary v-if="Component" title="页面渲染失败">
              <component :is="Component" :key="r.path" />
            </ErrorBoundary>
            <LoadingScreen v-else title="正在加载页面…" subtitle="正在准备页面组件与资源" />
          </router-view>
        </div>
      </el-main>
    </el-container>

    <!-- 手机浏览器底部专属轻量悬浮导航栏 (拇指秒切) -->
    <nav
      v-if="isMobile"
      class="fixed bottom-0 left-0 right-0 z-40 bg-white/95 dark:bg-[#13141b]/95 backdrop-blur-xl border-t border-gray-200/80 dark:border-white/10 px-1.5 py-1.5 flex items-center justify-around shadow-2xl select-none"
    >
      <router-link
        v-for="item in menuItems"
        :key="item.index"
        :to="item.index"
        class="flex flex-col items-center justify-center flex-1 py-1 px-1 rounded-xl text-[11px] font-medium transition-all"
        :class="[
          activePath === item.index
            ? 'text-indigo-600 dark:text-indigo-400 font-bold scale-105 bg-indigo-50/70 dark:bg-indigo-950/40'
            : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 active:scale-95'
        ]"
      >
        <div class="relative flex items-center justify-center">
          <el-icon :size="20" class="mb-0.5">
            <component :is="item.icon" />
          </el-icon>
          <span
            v-if="getMenuBadge(item.index) > 0"
            class="absolute -top-1.5 -right-2.5 min-w-[15px] h-3.5 px-1 rounded-full bg-rose-500 text-white text-[9px] font-bold flex items-center justify-center leading-none shadow-xs"
          >
            {{ getMenuBadge(item.index) > 99 ? '99+' : getMenuBadge(item.index) }}
          </span>
        </div>
        <span class="leading-none tracking-tight">{{ item.shortLabel || item.label }}</span>
      </router-link>
    </nav>

    <DebugPanel v-model="debugOpen" />
    <IncomingCallModal />
    <UserGuideDialog v-model="guideOpen" />
  </el-container>
</template>

<style scoped>
.sidebar-shell {
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  -webkit-font-smoothing: antialiased;
  text-rendering: optimizeLegibility;
  --sidebar-menu-text: #475569;
  --sidebar-menu-hover-bg: rgba(91, 91, 214, 0.08);
  --sidebar-menu-active-bg: linear-gradient(135deg, rgba(91, 91, 214, 0.14), rgba(91, 91, 214, 0.1));
  --sidebar-menu-active-color: #4a4ac2;
  --sidebar-menu-active-ring: rgba(91, 91, 214, 0.16);
}

.sidebar-brand-title {
  font-family: "Space Grotesk", "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 1.62rem;
  font-weight: 600;
  letter-spacing: -0.03em;
  line-height: 1;
  display: flex;
  align-items: center;
  min-height: 1.75rem;
  background: linear-gradient(135deg, #5b5bd6, #4a4ac2);
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  filter: drop-shadow(0 2px 8px rgba(91, 91, 214, 0.18));
  white-space: nowrap;
  padding-right: 4px;
}

.sidebar-brand-icon {
  width: 1.62rem;
  height: 1.62rem;
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: linear-gradient(135deg, #5b5bd6, #4a4ac2);
  color: #fff;
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-size: 0.84rem;
  font-weight: 700;
  box-shadow: 0 6px 14px rgba(91, 91, 214, 0.18);
}

.sidebar-menu-label {
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  font-weight: 500;
  letter-spacing: -0.01em;
}

:global(html.dark .sidebar-shell) {
  --sidebar-menu-text: rgba(255, 255, 255, 0.84);
  --sidebar-menu-hover-bg: rgba(91, 91, 214, 0.18);
  --sidebar-menu-active-bg: linear-gradient(135deg, rgba(91, 91, 214, 0.24), rgba(91, 91, 214, 0.15));
  --sidebar-menu-active-color: #a5a6f6;
  --sidebar-menu-active-ring: rgba(91, 91, 214, 0.22);
}

:deep(.sidebar-menu) {
  border-right: 0 !important;
  --el-menu-hover-bg-color: var(--sidebar-menu-hover-bg);
  --el-menu-active-color: var(--sidebar-menu-active-color);
  --el-menu-text-color: var(--sidebar-menu-text);
}

:deep(.sidebar-menu .el-menu-item) {
  height: 40px;
  min-height: 40px;
  line-height: 40px;
  margin: 2px 8px;
  border-radius: 10px;
  padding-left: 13px !important;
  padding-right: 13px !important;
  font-size: 0.94rem;
  font-weight: 400;
  letter-spacing: 0;
  color: var(--sidebar-menu-text);
  transition: background-color 160ms ease, color 160ms ease, box-shadow 160ms ease;
}

:deep(.sidebar-menu .el-menu-item .el-icon) {
  margin-right: 8px !important;
  font-size: 1.18rem;
  color: inherit;
}

:deep(.sidebar-menu .el-menu-item .el-icon svg) {
  width: 1.18rem;
  height: 1.18rem;
}

:deep(.sidebar-menu .el-menu-item:hover) {
  background: var(--sidebar-menu-hover-bg);
}

:global(html.dark .sidebar-shell .sidebar-menu .el-menu-item:not(.is-active)) {
  color: rgba(236, 241, 252, 0.9) !important;
}

:global(html.dark .sidebar-shell .sidebar-menu .el-menu-item:not(.is-active) .el-icon),
:global(html.dark .sidebar-shell .sidebar-menu .el-menu-item:not(.is-active) .sidebar-menu-label) {
  color: rgba(236, 241, 252, 0.9) !important;
}

:deep(.sidebar-menu .el-menu-item.is-active) {
  background: var(--sidebar-menu-active-bg);
  color: var(--sidebar-menu-active-color);
  box-shadow: inset 0 0 0 1px var(--sidebar-menu-active-ring);
}

:deep(.sidebar-menu .el-menu-item.is-active .el-icon),
:deep(.sidebar-menu .el-menu-item.is-active .sidebar-menu-label) {
  color: inherit;
}

:deep(.sidebar-menu .el-menu-item::after) {
  display: none !important;
}

:deep(.sidebar-menu.el-menu--collapse) {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
}

:deep(.sidebar-menu.el-menu--collapse .el-menu-item) {
  width: 36px;
  height: 36px;
  min-height: 36px;
  line-height: 36px;
  margin: 3px auto;
  border-radius: 10px;
  display: grid;
  place-items: center;
  padding: 0 !important;
}

:deep(.sidebar-menu.el-menu--collapse .el-menu-item .el-icon) {
  width: 1.18rem;
  height: 1.18rem;
  margin: 0 !important;
  font-size: 1.18rem;
  line-height: 1;
  display: grid;
  place-items: center;
}

:deep(.sidebar-menu.el-menu--collapse .el-menu-item .el-icon svg) {
  width: 1.18rem;
  height: 1.18rem;
  display: block;
}

:deep(.sidebar-menu.el-menu--collapse .el-menu-item .el-menu-tooltip__trigger) {
  position: static;
  inset: auto;
  width: 100%;
  height: 100%;
  padding: 0 !important;
  display: grid;
  place-items: center;
}

:deep(.sidebar-menu.el-menu--collapse > .el-menu-item [class^=el-icon]) {
  width: 1.18rem !important;
}

:deep(.sidebar-menu.el-menu--collapse .el-tooltip) {
  width: 36px;
  display: grid;
  place-items: center;
}

:deep(.sidebar-menu.el-menu--collapse .el-tooltip__trigger) {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
}

.main-inner {
  max-width: 100%;
}

@media (min-width: 768px) {
  .main-inner {
    max-width: clamp(0px, calc(100vw - 240px - 48px), 80rem);
  }
}

:deep(.mobile-drawer .el-drawer__body) {
  padding: 0 !important;
}
</style>
