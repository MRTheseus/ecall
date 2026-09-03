<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Rocket20Regular,
  Phone20Regular,
  Code20Regular,
  Sim20Regular,
  Globe20Regular,
  Mail20Regular,
  Call20Regular,
  QuestionCircle20Regular,
  Search20Regular,
  Copy20Regular,
  Checkmark20Regular,
  ShieldCheckmark20Regular,
  DocumentText20Regular
} from '@vicons/fluent'

const visible = defineModel<boolean>({ default: false })
const activeTab = ref('quickstart')
const searchQuery = ref('')
const copiedCommand = ref<string | null>(null)

function openAPIDocs() {
  window.open('/swagger/index.html', '_blank')
}

function copyText(text: string) {
  if (!navigator.clipboard) {
    ElMessage.error('当前浏览器环境不支持自动复制')
    return
  }
  navigator.clipboard.writeText(text).then(() => {
    copiedCommand.value = text
    ElMessage.success('已复制到剪贴板')
    setTimeout(() => {
      if (copiedCommand.value === text) {
        copiedCommand.value = null
      }
    }, 2000)
  }).catch(() => {
    ElMessage.error('复制失败')
  })
}

const tabs = [
  { id: 'quickstart', label: '快速上手', icon: Rocket20Regular },
  { id: 'devices', label: '模组管理', icon: Phone20Regular },
  { id: 'at_ussd', label: 'AT与USSD', icon: Code20Regular },
  { id: 'esim', label: 'eSIM管理', icon: Sim20Regular },
  { id: 'proxy', label: '代理与网络', icon: Globe20Regular },
  { id: 'sms_notify', label: '短信与通知', icon: Mail20Regular },
  { id: 'voice', label: '语音通话', icon: Call20Regular },
  { id: 'faq', label: '故障排查', icon: QuestionCircle20Regular }
]

const atCommands = [
  { cmd: 'AT+CSQ', desc: '查询信号强度（返回 0-31 之间的 RSSI 值，99 表示无信号）' },
  { cmd: 'AT+CREG?', desc: '查询 CS 域网络注册状态（1/5 表示已注册本地/漫游网络）' },
  { cmd: 'AT+CGREG?', desc: '查询 PS 数据域注册状态（1/5 表示数据网络就绪）' },
  { cmd: 'AT+QNWINFO', desc: '移远模组：查询当前驻留网络制式、频段(Band)、PLMN' },
  { cmd: 'AT+QENG="servingcell"', desc: '移远模组：获取详细服务小区物理参数（RSRP/RSRQ/SINR/PCI）' },
  { cmd: 'AT+QCFG="band"', desc: '移远模组：查询/配置锁定频段掩码' },
  { cmd: 'AT+CFUN=0 / AT+CFUN=1', desc: '切换最小功能（飞行模式）/ 开启全功能重连基站' },
  { cmd: 'AT+CFUN=1,1', desc: '软重启 Modem 模组芯片' },
  { cmd: 'AT+CPIN?', desc: '查询 SIM 卡状态（READY 为正常就绪）' },
  { cmd: 'AT+CIMI', desc: '读取 SIM 卡的国际移动用户识别码 (IMSI)' },
  { cmd: 'AT+CCID', desc: '读取 SIM/eSIM 卡的 ICCID 卡号' }
]

const faqs = [
  {
    q: 'Q1: 插入模组后在 Web 界面没有显示设备怎么办？',
    a: '1. 检查供电：多模组并发运行时请务必使用带独立电源供电的 USB HUB（推荐 5V/3A 或更高规格）。\n2. 检查 USB 驱动：在宿主机终端执行 `ls /dev/ttyUSB*` 或 `lsusb`，确认系统已识别到移远模组虚拟串口（通常为 4~5 个 ttyUSB 节点）。\n3. Docker 权限：如果使用 Docker 部署，请确保 `docker-compose.yml` 中配置了 `privileged: true` 并将 `/dev` 设备目录完整挂载。'
  },
  {
    q: 'Q2: 模组显示有信号但无法拨号上网或无法获取 IP？',
    a: '1. 检查 APN：进入「设备管理」→「配置」页，确认 APN 是否与您的 SIM 卡运营商匹配（如国内常见：中国移动 `cmnet`、中国联通 `3gnet`、中国电信 `ctnet`；物联卡/海外卡请填写专用 APN）。\n2. 检查 IP 版本：部分运营商仅支持 IPv4 或 IPv4v6，请在配置项中切换 IP 版本尝试。\n3. 检查 SIM 状态：确认 SIM 卡未欠费、未停机且未锁定 PIN 码。'
  },
  {
    q: 'Q3: eSIM 下载安装 Profile 提示失败如何处理？',
    a: '1. 确保激活码正确：Activation Code 格式通常为 `LPA:1$smdp.example.com$MATCHING-ID`。\n2. 确保网络通畅：下载 Profile 需模组当前有蜂窝网络或宿主机具备可用公网连接以访问 SM-DP+ 服务器。\n3. 确保芯片接触良好：板载芯片或外置 9eSIM 实体卡需确保金手指良好接触且支持 GSMA SGP.22 / SGP.32 规范。'
  },
  {
    q: 'Q4: SOCKS5 / HTTP 代理连接超时或无法访问特定网站？',
    a: '1. 端口放行：检查宿主机防火墙（ufw / iptables）是否放行了所配置的代理端口。\n2. 模组出站绑定：Ecall 基于 Linux 内核 `SO_BINDTODEVICE` 机制将代理严格绑定到模组网卡（如 wwan0）。请确保该模组当前已成功获取蜂窝 IP。\n3. DNS 解析：若蜂窝网络提供的 DNS 存在污染或延迟，可尝试在模组设置中配置公共 DNS（如 119.29.29.29 或 8.8.8.8）。'
  },
  {
    q: 'Q5: 如何实现自动 IP 轮换（切换公网出口 IP）？',
    a: '1. 手动轮换：在「设备管理」或「代理管理」中点击「刷新 IP / 重连」，系统将自动触发飞行模式重连基站，运营商将重新分配蜂窝 IP。\n2. Telegram 远程轮换：配置好 TG Bot 后，向 Bot 发送 `/rotate <设备ID>` 即可远程一键刷新 IP。\n3. API 轮换：调用 `/api/devices/{id}/restart` 或重拨接口即可实现程序化调度轮换。'
  }
]

const filteredFaqs = computed(() => {
  if (!searchQuery.value.trim()) return faqs
  const q = searchQuery.value.trim().toLowerCase()
  return faqs.filter(item => item.q.toLowerCase().includes(q) || item.a.toLowerCase().includes(q))
})

const filteredAtCommands = computed(() => {
  if (!searchQuery.value.trim()) return atCommands
  const q = searchQuery.value.trim().toLowerCase()
  return atCommands.filter(item => item.cmd.toLowerCase().includes(q) || item.desc.toLowerCase().includes(q))
})
</script>

<template>
  <el-dialog
    v-model="visible"
    title="Ecall 系统使用指南与操作手册"
    width="92%"
    class="user-guide-dialog !max-w-4xl !rounded-2xl"
    destroy-on-close
    append-to-body
  >
    <div class="guide-container flex flex-col h-[68vh] min-h-[480px]">
      <!-- 顶部搜索栏与简介 -->
      <div class="guide-header pb-3 mb-3 border-b border-gray-100 dark:border-white/10 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          基于多模组热插拔、`SO_BINDTODEVICE` 严格绑网与 LPA eSIM 的全功能移动通信平台
        </div>
        <div class="w-full sm:w-64">
          <el-input
            v-model="searchQuery"
            placeholder="搜索指令、问题或功能…"
            size="small"
            clearable
          >
            <template #prefix>
              <el-icon class="text-gray-400"><Search20Regular /></el-icon>
            </template>
          </el-input>
        </div>
      </div>

      <!-- 主体分栏：左侧导航 / 右侧详情 -->
      <div class="guide-body flex-1 flex flex-col sm:flex-row overflow-hidden gap-4">
        <!-- 导航菜单 -->
        <div class="guide-nav sm:w-44 flex-shrink-0 flex sm:flex-col overflow-x-auto sm:overflow-y-auto space-x-1 sm:space-x-0 sm:space-y-1 pb-2 sm:pb-0 pr-1">
          <button
            v-for="t in tabs"
            :key="t.id"
            @click="activeTab = t.id"
            class="flex items-center gap-2 px-3 py-2 rounded-xl text-xs font-medium transition-all text-left whitespace-nowrap cursor-pointer"
            :class="activeTab === t.id
              ? 'bg-indigo-50 dark:bg-indigo-500/15 text-indigo-600 dark:text-indigo-300 font-semibold shadow-xs'
              : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-white/5'"
          >
            <el-icon :size="16"><component :is="t.icon" /></el-icon>
            <span>{{ t.label }}</span>
          </button>
        </div>

        <!-- 内容滚动区域 -->
        <div class="guide-content flex-1 overflow-y-auto pr-2 text-sm space-y-4 text-gray-700 dark:text-gray-200">
          
          <!-- 1. 快速上手 -->
          <div v-show="activeTab === 'quickstart'" class="space-y-4">
            <div class="p-4 rounded-xl bg-gradient-to-br from-indigo-500/10 via-purple-500/5 to-transparent border border-indigo-500/20">
              <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
                <el-icon class="text-indigo-500"><Rocket20Regular /></el-icon>
                欢迎使用 Ecall
              </h3>
              <p class="mt-1 text-xs text-gray-600 dark:text-gray-300 leading-relaxed">
                Ecall 专为高通/移远 4G/5G 模组（EC20、EC25、EG25、EM20、RM500Q 等）设计，整合模组热插拔管理、独立网卡绑定 SOCKS5/HTTP 代理、短信智能收发、eSIM 全生命周期管理及全渠道告警推送。
              </p>
            </div>

            <div class="space-y-3">
              <h4 class="font-bold text-gray-900 dark:text-white">🚀 初次配置指引</h4>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">1. 安全与密码修改</div>
                  <div class="text-gray-500 dark:text-gray-400">系统内置默认账户为 <code class="px-1 py-0.5 rounded bg-gray-200 dark:bg-gray-800">admin / admin</code>（Docker 部署可在启动时通过环境变量 <code class="px-1 py-0.5 rounded bg-gray-200 dark:bg-gray-800">-e PROXY_WEB_PASSWORD=xxx</code> 自定义密码；未传时首次启动会在容器日志中随机打印一次性密码）。登录后请前往「系统设置」修改密码。</div>
                </div>
                <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">2. 模组接入与识别</div>
                  <div class="text-gray-500 dark:text-gray-400">插入 USB 模组后系统后台自动识别 ttyUSB/wwan 网卡，并在「设备管理」即时展示状态卡片。</div>
                </div>
                <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">3. 开启独立代理</div>
                  <div class="text-gray-500 dark:text-gray-400">前往「代理管理」为每个模组配置独立的 HTTP/SOCKS5 端口与认证，实现单机多出口代理池。</div>
                </div>
                <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">4. 配置消息推送</div>
                  <div class="text-gray-500 dark:text-gray-400">在「系统设置」中配置 Telegram Bot、Bark、飞书或 Webhook，接收短信与异常告警。</div>
                </div>
              </div>
            </div>

            <!-- 电信与短信风控反滥用机制 (Anti-Abuse Rate Limiting) -->
            <div class="p-4 rounded-xl bg-indigo-50/70 dark:bg-indigo-500/10 border border-indigo-200/60 dark:border-indigo-500/20 space-y-2.5">
              <div class="flex items-center gap-2 font-bold text-gray-900 dark:text-white text-xs">
                <el-icon class="text-indigo-600 dark:text-indigo-400"><ShieldCheckmark20Regular /></el-icon>
                <span>🛡️ 电信与短信风控反滥用机制 (Anti-Abuse Rate Limiting)</span>
              </div>
              <p class="text-xs text-gray-600 dark:text-gray-300 leading-relaxed">
                为防止因脚本异常或恶意调用导致 SIM 卡资费被瞬时盗刷，并严格杜绝短信群发、电信违规滥用行为，系统内置了电信级安全频控策略：
              </p>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-2.5 text-xs">
                <div class="p-2.5 rounded-lg bg-white/80 dark:bg-white/5 border border-indigo-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-0.5">⏱️ 冷启动保护 (首小时限 3 条)</div>
                  <div class="text-gray-500 dark:text-gray-400">服务进程刚启动或重启的前 1 小时内，全局最多允许发送 3 条短信，阻断因程序循环漏洞或外部恶意探测瞬间刷爆 SIM 卡资费。</div>
                </div>
                <div class="p-2.5 rounded-lg bg-white/80 dark:bg-white/5 border border-indigo-100 dark:border-white/5">
                  <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-0.5">📅 每日配额保护 (每日限 10 条)</div>
                  <div class="text-gray-500 dark:text-gray-400">单个自然日（次日 00:00 自动重置）全局短信发送上限为 10 条，确保平台仅用于合规个人测试与轻量运维告警，彻底杜绝群发风险。</div>
                </div>
              </div>
              <div class="text-[11px] text-gray-500 dark:text-gray-400">
                💡 触发风控限流时，API 与 Web 界面将明确返回错误提示与预计恢复时间（Retry-After），保障底层硬件与 SIM 卡通信状态稳定。
              </div>
            </div>

            <div class="p-3 rounded-xl bg-amber-50 dark:bg-amber-500/10 border border-amber-200/60 dark:border-amber-500/20 text-xs text-amber-800 dark:text-amber-300">
              <span class="font-bold">⚠️ 注意事项：</span>本软件仅供个人技术研究与内部测试，严禁用于任何商业用途或违反电信法规的场景。
            </div>
          </div>

          <!-- 2. 模组管理 -->
          <div v-show="activeTab === 'devices'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Phone20Regular /></el-icon>
              模组状态与网络策略
            </h3>

            <div class="space-y-2 text-xs">
              <div class="font-semibold text-gray-800 dark:text-gray-200">📊 关键指标解读：</div>
              <ul class="list-disc pl-5 space-y-1 text-gray-600 dark:text-gray-300">
                <li><strong>RSSI：</strong>接收信号强度指示。通常 -50 ~ -75 dBm 为极佳，低于 -90 dBm 信号较弱。</li>
                <li><strong>RSRP：</strong>参考信号接收功率（LTE/5G）。-80 dBm 以上极佳，-100 dBm 以下易出现丢包。</li>
                <li><strong>SINR：</strong>信噪比。数值越高抗干扰能力越强，通常 &gt; 15 dB 速率最佳。</li>
                <li><strong>网卡 (Interface)：</strong>底层绑定的网络接口（如 `wwan0`, `usb0`），出站流量通过此接口严格流转。</li>
              </ul>
            </div>

            <div class="space-y-2 text-xs">
              <div class="font-semibold text-gray-800 dark:text-gray-200">⚙️ 卡策略配置与锁网：</div>
              <p class="text-gray-600 dark:text-gray-300">
                在设备详情页可针对单张 SIM 卡/模组定制卡策略：
              </p>
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-2">
                <div>🔹 <strong>锁频段 (Band Lock)：</strong>锁定特定 LTE 频段（如 B1, B3, B5, B8, B38, B40 等），防止基站拥堵时频繁跳频。</div>
                <div>🔹 <strong>网络制式锁定：</strong>可选择「仅限 4G」、「仅限 5G」或「自动选网」，避免网络掉落至 2G/3G 影响业务。</div>
                <div>🔹 <strong>运营商手动选网 (PLMN)：</strong>支持搜索周边基站并强制注册到指定 PLMN（如 46000 移动、46001 联通、46011 电信）。</div>
              </div>
            </div>
          </div>

          <!-- 3. AT与USSD -->
          <div v-show="activeTab === 'at_ussd'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Code20Regular /></el-icon>
              AT 指令终端与 USSD 交互
            </h3>

            <div class="text-xs text-gray-600 dark:text-gray-300">
              Ecall 内置了完整的 AT 指令交互终端，支持常用指令一键发送与原始响应查看：
            </div>

            <div class="space-y-2">
              <div class="text-xs font-bold text-gray-800 dark:text-gray-200">常用 AT 指令速查（点击右侧可一键复制）：</div>
              <div class="space-y-1.5 max-h-60 overflow-y-auto pr-1">
                <div
                  v-for="item in filteredAtCommands"
                  :key="item.cmd"
                  class="flex items-center justify-between p-2 rounded-lg bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 text-xs hover:border-indigo-200 dark:hover:border-indigo-500/30 transition-colors"
                >
                  <div class="flex-1 min-w-0 pr-2">
                    <span class="font-mono font-bold text-indigo-600 dark:text-indigo-400">{{ item.cmd }}</span>
                    <span class="text-gray-500 dark:text-gray-400 ml-2">{{ item.desc }}</span>
                  </div>
                  <button
                    @click="copyText(item.cmd)"
                    class="p-1 rounded text-gray-400 hover:text-indigo-600 dark:hover:text-indigo-300 transition-colors cursor-pointer"
                    title="复制指令"
                  >
                    <el-icon :size="14">
                      <Checkmark20Regular v-if="copiedCommand === item.cmd" class="text-emerald-500" />
                      <Copy20Regular v-else />
                    </el-icon>
                  </button>
                </div>
              </div>
            </div>

            <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 text-xs space-y-1.5">
              <div class="font-bold text-indigo-600 dark:text-indigo-400">📱 USSD 交互中心用法：</div>
              <div class="text-gray-600 dark:text-gray-300">输入运营商 USSD 代码（例如：`*100#` 查询余额，`*101#` 查询套餐流量），系统自动完成 7-bit / UCS2 编码转换并显示中文/英文回执信息。</div>
            </div>
          </div>

          <!-- 4. eSIM 管理 -->
          <div v-show="activeTab === 'esim'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Sim20Regular /></el-icon>
              eSIM 芯片与 Profile 生命周期管理
            </h3>

            <div class="text-xs text-gray-600 dark:text-gray-300">
              Ecall 支持直接通过 Modem AT 指令通道对 eSIM 芯片（如 9eSIM、Estk、5ber、LPAC 兼容芯片）执行全生命周期管理：
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">📥 Profile 在线下载</div>
                <div class="text-gray-500 dark:text-gray-400">支持输入 LPA 激活码（格式：<code>LPA:1$smdp.domain$MATCHING-ID</code>）直接从运营商 SM-DP+ 服务器下载并安装 Profile。</div>
              </div>
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">🔄 一键切换激活</div>
                <div class="text-gray-500 dark:text-gray-400">在多个已安装的卡号之间平滑切换生效 Profile，无需插拔物理卡。</div>
              </div>
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">✏️ 别名与重命名</div>
                <div class="text-gray-500 dark:text-gray-400">为每个 Profile 设定清晰的自定义别名（如「香港宽频 50G」、「欧洲漫游卡」），便于管理。</div>
              </div>
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400 mb-1">🗑️ 安全删除与清理</div>
                <div class="text-gray-500 dark:text-gray-400">支持释放废弃 Profile 存储空间并向服务器发送注销通知。</div>
              </div>
            </div>
          </div>

          <!-- 5. 代理与网络 -->
          <div v-show="activeTab === 'proxy'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Globe20Regular /></el-icon>
              轻量级代理引擎与私有代理池编排
            </h3>

            <div class="p-3 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 border border-indigo-200/50 dark:border-indigo-500/20 text-xs space-y-1">
              <div class="font-bold text-indigo-600 dark:text-indigo-400">💡 核心技术原理：`SO_BINDTODEVICE` 强绑网卡</div>
              <div class="text-gray-600 dark:text-gray-300">
                Ecall 在 Socket 握手层调用 Linux 内核的 <code>SO_BINDTODEVICE</code>，将每个代理端口的 TCP/UDP 连接严格绑定到对应模组的蜂窝网卡上，彻底避免了流量串流与路由混乱，实现真正意义上的「单机多卡、一端口一出口 IP」。
              </div>
            </div>

            <div class="space-y-2 text-xs">
              <div class="font-semibold text-gray-800 dark:text-gray-200">🛠️ 代理配置要点：</div>
              <ul class="list-disc pl-5 space-y-1.5 text-gray-600 dark:text-gray-300">
                <li><strong>协议支持：</strong>同时支持 SOCKS5 与 HTTP 代理协议，可为各端口独立启用。</li>
                <li><strong>安全认证：</strong>支持配置专属的用户名/密码，防止代理被未授权扫描和滥用。</li>
                <li><strong>IP 轮换机制：</strong>支持在界面点击「重置 IP / 切换蜂窝连接」，模组将自动触发快速飞行模式重连基站，获取运营商分配的新公网出口 IP。</li>
                <li><strong>流量看板：</strong>实时监控每个代理通道的瞬时上行/下行速率及历史流量消耗统计。</li>
              </ul>
            </div>
          </div>

          <!-- 6. 短信与通知 -->
          <div v-show="activeTab === 'sms_notify'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Mail20Regular /></el-icon>
              短信收发中心与全渠道告警推送
            </h3>

            <div class="space-y-3 text-xs">
              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200 mb-1">📩 短信智能处理：</div>
                <div class="text-gray-600 dark:text-gray-300">
                  支持 PDU 与 Text 格式短信发送；接收到的长短信自动合并拼接，自动高亮提取短信验证码，并持久化保存在本地 SQLite 数据库中供随时检索。
                </div>
              </div>

              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200 mb-1">🤖 Telegram Bot 远程交互命令：</div>
                <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1 font-mono text-[11px]">
                  <div><code>/list</code> - 列出所有在线模组与状态</div>
                  <div><code>/rotate &lt;设备ID&gt;</code> - 远程重置并刷新指定模组蜂窝 IP</div>
                  <div><code>/sms &lt;设备ID&gt;</code> - 查看指定模组最近接收的短信</div>
                  <div><code>/send &lt;设备ID&gt; &lt;号码&gt; &lt;内容&gt;</code> - 远程通过指定卡发送短信</div>
                </div>
              </div>

              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200 mb-1">🔔 支持的多渠道推送平台：</div>
                <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">Telegram Bot</span>
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">Bark (iOS)</span>
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">飞书 / Lark</span>
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">QQ (OneBot/Qmsg)</span>
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">PushPlus (微信)</span>
                  <span class="p-2 rounded-lg bg-gray-50 dark:bg-white/5 text-center text-gray-600 dark:text-gray-300">自定义 Webhook / 邮件</span>
                </div>
              </div>

              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">🛡️ 短信外发风控规则：</div>
                <div class="text-gray-600 dark:text-gray-300">
                  为保障 SIM 卡安全并防范短信盗刷，系统默认实施启动首小时限发 3 条、每日限发 10 条的安全阈值防护。
                </div>
              </div>
            </div>
          </div>

          <!-- 7. 语音通话 -->
          <div v-show="activeTab === 'voice'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><Call20Regular /></el-icon>
              语音通话与 VoWiFi / IMS 状态
            </h3>

            <div class="space-y-3 text-xs text-gray-600 dark:text-gray-300">
              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1.5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">📞 Web 拨号盘与来电弹窗：</div>
                <div>支持在浏览器内直接输入电话号码发起语音呼叫，来电时系统自动弹出接听/挂断提示框。</div>
              </div>

              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1.5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">🔢 DTMF 智能按键：</div>
                <div>在呼叫客服电话（如 10086 / 10010）时，可使用界面上的数字键盘实时发送 DTMF 双音多频信号完成语音菜单交互导航。</div>
              </div>

              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-gray-800 space-y-1.5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">🎙️ 双向通话录音与在线试听/下载：</div>
                <div>支持在通话过程中一键开启双向实时录音，采用服务端 PCM 混音与 FFmpeg 高保真压缩存储为标准 MP3 格式。通话记录列表支持一键在线播放试听与文件下载。</div>
              </div>

              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-gray-800 space-y-1.5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">⚡ 高清低延迟 WebRTC 媒体直连：</div>
                <div>底层通过 ALSA 硬件驱动与 Pion WebRTC 管道直连，内置局域网与物理网卡动态自适应过滤，支持快速挂断重拨无缝切换，以及浏览器后台标签页静默保活。</div>
              </div>

              <div class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1.5">
                <div class="font-bold text-indigo-600 dark:text-indigo-400">📶 VoWiFi 零信号通信：</div>
                <div>支持模组通过宽带互联网建立 IPSec 隧道接入运营商核心网 IMS，在地下室等弱信号环境下保持通话畅通。</div>
              </div>
            </div>
          </div>

          <!-- 8. 故障排查 -->
          <div v-show="activeTab === 'faq'" class="space-y-4">
            <h3 class="text-base font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <el-icon class="text-indigo-500"><QuestionCircle20Regular /></el-icon>
              高频问题解答与排障手册 (FAQ)
            </h3>

            <div class="space-y-3">
              <div
                v-for="f in filteredFaqs"
                :key="f.q"
                class="p-3 rounded-xl bg-gray-50 dark:bg-white/5 border border-gray-100 dark:border-white/5 space-y-1.5"
              >
                <div class="text-xs font-bold text-indigo-600 dark:text-indigo-400">{{ f.q }}</div>
                <div class="text-xs text-gray-600 dark:text-gray-300 leading-relaxed whitespace-pre-line">{{ f.a }}</div>
              </div>
            </div>
          </div>

        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 text-xs text-gray-400 dark:text-gray-500 w-full pt-2 border-t border-gray-100 dark:border-white/5">
        <div class="flex items-center gap-2">
          <span class="font-semibold text-gray-600 dark:text-gray-400">Ecall 蜂窝移动通信与模组控制系统</span>
          <span>·</span>
          <span class="font-mono bg-indigo-50 dark:bg-indigo-950/40 text-indigo-600 dark:text-indigo-400 px-2 py-0.5 rounded-full text-[11px] font-bold">v1.0.0</span>
        </div>
        <div class="flex items-center gap-3">
          <el-button size="small" type="primary" plain @click="openAPIDocs">
            <el-icon class="mr-1"><DocumentText20Regular /></el-icon>
            OpenAPI 接口文档
          </el-button>
          <el-button size="small" @click="visible = false">关闭</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.guide-container {
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
}

.guide-content::-webkit-scrollbar,
.guide-nav::-webkit-scrollbar {
  width: 5px;
  height: 5px;
}

.guide-content::-webkit-scrollbar-thumb,
.guide-nav::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 4px;
}

.dark .guide-content::-webkit-scrollbar-thumb,
.dark .guide-nav::-webkit-scrollbar-thumb {
  background: #334155;
}
</style>
