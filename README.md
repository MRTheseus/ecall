# VoHive

[![License: PolyForm Noncommercial 1.0.0](https://img.shields.io/badge/License-PolyForm--Noncommercial--1.0.0-blue.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](go.mod)
[![Vue 3](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js)](web/package.json)

> 面向高通 4G/LTE 蜂窝模组的综合管理、SOCKS5/HTTP 代理池与**浏览器原生 VoLTE 实时语音通话**服务平台。

本项目基于开源项目 **[VoHive (by iniwex5)](https://github.com/iniwex5/vohive)** 进行深度定制与拓展开发，在原有模组热插拔管理、多网卡代理编排、短信中心与 eSIM 管理的基础上，全新实现了**全链路浏览器 WebRTC ⇋ 模组 VoLTE 双向低延迟实时语音通话**，并针对大疆 DJI Cellular 模块等高通 MDM9207 硬件提供了免拆机、免刷机的全自动声卡与 QDSP6 语音路由注入托管。

---

## 核心特性

| 模块 | 说明 |
| --- | --- |
| **VoLTE 实时语音通话 (NEW)** | 网页端原生 WebRTC 双向通话：支持拨号盘呼出、来电全局弹窗振铃与接听、挂断、DTMF 按键交互、麦克风静音切换、实时通话计时 |
| **模组语音运行时全自动托管 (NEW)** | 内置大疆/高通 AT 挑战码解密，**免拆机全自动激活 USB ADB**，自动热注入专版 APR 语音驱动与 QDSP6 Mixer 混音矩阵，真正做到即插即用 |
| **超低延迟转码引擎 (NEW)** | 基于 Pion WebRTC v4 与毫秒级实时 FFmpeg 流水线，实现 Opus 48000Hz ⇋ VoLTE PCM 8000Hz 零积压实时全双工双向互通 |
| **多模组并发管理** | USB 热插拔自动发现 (`ttyUSB*`、`cdc-wdm*`)、实时信号强度 (CSQ/RSRP/RSRQ)、频段与网络状态监控 |
| **轻量级代理引擎** | 内建 SOCKS5 / HTTP 代理内核，支持多实例并发；基于 `SO_BINDTODEVICE` 按设备网卡严格绑定移动蜂窝出站流量 |
| **通信与短信中心** | 统一 Web 界面与 API 处理短信收发、长短信自动重组、会话与联系人管理、USSD 交互，短信持久化落库 |
| **eSIM 管理** | 通过 AT 指令通道直接管理 LPA/eSIM 芯片，支持 Profile 下载、启用/停用、重命名、删除 |
| **全渠道通知中心** | 短信及系统事件可自动推送到 Telegram、Email、PushPlus、Bark、飞书(Lark)、QQ 等渠道 |
| **全平台跨架构** | 原生支持 amd64 / arm64 / armv7 容器化编译与运行，工控机、软路由（PVE / iStoreOS / OpenWrt）到边缘节点均可稳定运行 |

---

---

## 4G 模组选型与兼容性判断指南

在使用本项目前，请根据下表判断您的 4G 模组是否支持所需功能（特别是 **VoLTE 实时网页语音通话**）：

### 1. 模组功能支持快速速查表

| 功能特性 | 硬件与固件核心要求 | 代表模组与适配情况 |
| :--- | :--- | :--- |
| **基础代理池 (SOCKS5/HTTP)** | 支持 Linux `cdc_wdm` (QMI) 或 `cdc_mbim` (MBIM) 协议 | 绝大多数高通/ASR 4G 模组均支持 (大疆、移远 EC20/25、广和通等) |
| **短信中心收发与转发** | 支持标准 3GPP AT 指令集 (`AT+CMGS` / `AT+CMGL`) 或 QMI WMS | 绝大多数 4G 模组均支持 |
| **eSIM / LPA 芯片管理** | 模组内置或外挂 eSIM 芯片，支持 LPA AT 指令集 | 大疆 Cellular 模组 (内置联通 eSIM)、部分移远 eSIM 定制版 |
| **VoLTE 网页实时语音通话** | **必须同时满足**：<br>① SIM 卡开通 VoLTE 并完成 IMS 注册；<br>② 模组必须在宿主机枚举出 **USB Audio (UAC) 声卡设备**；<br>③ 基带音频需与 USB 声卡完成内部数字音频路由桥接 | - **大疆 DJI Cellular (QDC507/IG830)**：✅ **完美支持** (本项目内置自动化挑战码解算与免拆 ADB 注入驱动)<br>- **移远 Quectel (EC20 / EC25 / EG25 等)**：✅ **支持** (需通过 AT 指令开启 USB 声卡模式)<br>- **无 USB 声卡的普通 4G 模组/随身WiFi**：❌ **不支持语音** (仅支持数据代理与短信) |

---

### 2. 如何快速检测自己的模组是否适合？（宿主机 3 步自查）

在 Linux 宿主机（或 PVE / 软路由终端）插入 4G 模块后，依次执行以下命令：

#### 步骤 1：检查 USB 通信节点
```bash
ls -l /dev/ttyUSB* /dev/cdc-wdm*
```
- **判定标准**：至少需要出现 2~3 个 `ttyUSB*`（分别作为 AT 指令口、语音串口等），以及 `cdc-wdm0`（QMI 通信口）。若仅有网卡无串口，可能需要通过 AT 指令切换 USB 模式。

#### 步骤 2：检查是否支持 USB 声卡（VoLTE 语音的核心硬指标！）
```bash
arecord -l && aplay -l
```
- **判定标准**：查看输出中是否有以 `USB Audio`、`Baiwang`、`Quectel` 等命名的声卡设备，例如：
  ```text
  card 0: Baiwang [Baiwang], device 0: USB Audio [USB Audio]
    Subdevices: 1/1
    Subdevice #0: subdevice #0
  ```
- **结论**：**只有输出中存在 USB Audio 声卡的模组，才具备接入 WebRTC 实时语音通话的物理条件**。如果只有纯数据节点而没有声卡，则该模块无法用于语音通话。

#### 步骤 3：检查 SIM 卡 VoLTE 与 IMS 驻网状态
在管理后台设备详情或通过串口调试工具向模组发送：
```text
AT+CEREG?       # 查询 4G LTE 驻网状态，返回 ,1 或 ,5 表示已正常注册
AT+CGDCONT?     # 查看 APN 配置，确保有用于 VoLTE 的 IMS APN (如 ims)
```

---

### 3. 主流模组详细配置与处理指南

#### A. 大疆 DJI Cellular 模块（第一代 4G Dongle）

- **硬件型号**：Baiwang / 百旺 QDC507 / IG830（高通 Qualcomm MDM9207，Linux 3.18.44）。
- **特点与处理说明**：
  - **大疆官方原厂固件（免拆机、免刷机）**：
    > 原厂固件默认屏蔽了 ADB 与底层语音通路。**本项目已内置大疆专有算法，无需用户做任何前置处理**！
    > 服务启动时会自动拦截 `AT+QADBKEY?` 挑战码、秒级算解 15 位密钥并激活第 7 位 ADB 调试通道，随后自动将语音运行时（`qdc507_aprv3.ko`、`qdc507_voice.ko`、QDSP6 混音矩阵与桥接器）热注入到模组底座中。
  - **第三方/已刷机固件**：
    > 完全兼容已开启 ADB 或刷入开放固件的大疆模组。
  - **自愈机制保障**：系统内嵌底座僵尸进程检测与冷启动强制清场机制，即使宿主机 Docker 频繁重启，模组底层音频依然保持自动解锁与自愈。

#### B. 移远 Quectel 系列模组（EC20 / EC25 / EG25 / EM20 等）

移远标准高通模组支持通过 AT 指令开启 USB 声卡功能：
1. **开启 USB Audio UAC 声卡模式**：
   使用串口工具向模组发送指令：
   ```text
   AT+QPCMV=1,2
   ```
   > 参数说明：`1` 表示启用声卡，`2` 表示选择 USB Audio 模式。设置后发送 `AT+CFUN=1,1` 重启模组生效。
2. **确认启用 VoLTE 与 IMS 支持**：
   ```text
   AT+QCFG="ims",1
   ```
   > 确保模组开启 IMS 栈，插入开通了 VoLTE 的 SIM 卡即可自动发起和接听 VoLTE 呼叫。
3. **开启呼叫识别 URC 上报**：
   ```text
   AT+CLIP=1;+COLP=1
   ```
   > 确保来电和呼出时模组能通过串口向 VoHive 实时上报电话号码及挂断原因。

#### C. 其他普通 4G 随身 WiFi / ASR 模组（无 USB 声卡硬件）

市面上大部分低成本随身 WiFi（如基于中兴微 ZX297520、展锐 UIS8910、ASR1802 等芯片的板子）：
- **现状**：硬件上并未向 USB 端引出 UAC 音频端点，或者固件基带中 VoLTE 语音通路仅支持耳机孔物理硬件排针输出；
- **用途定位**：此类模块**仅可用于 VoHive 的 4G 代理池构建、网页短信收发与通知推送**，无法直接使用 WebRTC 语音通话功能。

---

## 运行环境与部署要求

### 1. Docker 运行配置

为保证 WebRTC 媒体流与底层硬件声卡/串口设备的低延迟交互，Docker 容器必须配置以下参数：

```yaml
services:
  vohive:
    image: vohive:voice
    container_name: vohive
    restart: unless-stopped
    privileged: true              # 必须开启特权模式，用于访问 ALSA 声卡与 USB 节点
    network_mode: host            # 必须开启 host 网络模式，用于 WebRTC UDP 媒体流低延迟穿透
    volumes:
      - /dev:/dev                 # 必须映射宿主机 /dev 目录 (ttyUSB* / snd/* / cdc-wdm*)
      - ./data:/app/data
      - ./config:/app/config
      - ./logs:/app/logs
```

### 2. 浏览器 WebRTC 安全上下文限制说明

现代浏览器（Chrome、Edge、Safari）出于用户隐私安全策略，**仅允许在“安全上下文（Secure Context）”下调用麦克风**：

- **方式一（推荐）**：配置域名与 SSL/TLS 证书，通过 **HTTPS** 访问（例如反代到 `https://ecall.yourdomain.top:57575`），浏览器原生无缝允许麦克风权限；
- **方式二（局域网 HTTP 测试）**：若在局域网通过纯 HTTP（如 `http://192.168.1.100:7575`）访问，需在浏览器地址栏打开：
  `chrome://flags/#unsafely-treat-insecure-origin-as-secure`
  将您的 Web 访问地址（例如 `http://192.168.1.100:7575`）填入文本框，右侧设为 **Enabled** 并点击右下角 **Relaunch** 重启浏览器，即可解除局域网麦克风限制。

> [!TIP]
> 测试通话时，如果呼出端与接听端在同一房间，请插上耳机或点击界面上的“静音麦克风”按钮，避免扬声器与麦克风在物理空间形成自激啸叫导致浏览器回声消除引擎（AEC）自动抑制音量。

---

## 开源说明与致谢 (Acknowledgements)

本项目在开源社区众多优秀项目与逆向工程成果的肩膀上构建，特此致以诚挚谢意：

- **[VoHive](https://github.com/iniwex5/vohive) / [OpenVoHive](https://github.com/dannyge/openvohive)**：由 [iniwex5](https://github.com/iniwex5) 及开源社区维护的多模组管理与代理平台基础框架。
- **[Pion WebRTC](https://github.com/pion/webrtc)**：纯 Go 语言实现的高性能、低延迟 WebRTC 完整协议栈。
- **[FFmpeg](https://ffmpeg.org/)**：跨平台的音视频转码多媒体处理引擎，负责毫秒级 Opus ⇋ PCM 流式全双工编解码。
- **[ALSA (Advanced Linux Sound Architecture)](https://www.alsa-project.org/)**：Linux 内核级音频架构与 `alsa-utils` 录放音管线。
- **[Android Open Source Project (android-tools)](https://android.googlesource.com/platform/system/core/+/master/adb/)**：高通底座通信与驱动热加载所需的 ADB 工具链。
- **大疆硬件逆向研究社区**：提供了高通 MDM9207 芯片底座的 QDSP6 混音器架构、APR 驱动以及 `mavo-pcm-bridge`、`alsaucm_test` 的底层调用思路。

---

## 免责声明

- **用途定位**：本项目仅供个人技术测试、硬件研究、逆向工程学习与技术交流使用，严禁用于任何商业用途或非法违规场景；因使用本项目产生的一切硬件、资费或网络风险由使用者自行承担。
- **非官方声明**：本项目为独立开发研究成果，与大疆（DJI）、移远（Quectel）、高通公司（Qualcomm）及任何电信运营商均无任何官方关联、授权或背书关系。
- **合规声明**：使用本服务请务必遵守当地法律法规及运营商入网服务协议，严禁利用蜂窝网络进行骚扰电话、电信欺诈、未经授权的流量中继等行为。
- **无担保条款**：本软件按“现状”提供，作者及贡献者不对因使用本项目导致的软硬件损坏、数据丢失、网络阻断等后果承担任何明示或暗示的担保及连带责任。

---

## License

本项目基于 [PolyForm Noncommercial License 1.0.0](LICENSE) 协议开源，**仅限非商业用途**：
- 可自由用于个人学习、硬件研究、学术实验等非商业场景；
- **严格禁止任何形式的商业用途**（包括但不限于商业销售、收费代理托管服务、集成至盈利性产品）。

> Required Notice: Copyright iniwex5 (https://github.com/iniwex5/vohive) & Contributors.
