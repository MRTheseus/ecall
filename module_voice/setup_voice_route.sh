#!/bin/sh
set -e

# ==============================================================================
# QDC507 (MDM9207) 音频路由自愈管理脚本
# 支持 --reset / --force 参数：在服务冷启动时彻底清场并重建路由
# ==============================================================================

FORCE_RESET=0
if [ "$1" = "--reset" ] || [ "$1" = "--force" ]; then
    FORCE_RESET=1
fi

# 1. 驱动模块检查与加载
if ! grep -q 'qdc507_voice' /proc/modules; then
    insmod /tmp/djonehub-call/qdc507_aprv3.ko 2>/dev/null || true
    insmod /tmp/djonehub-call/qdc507_voice.ko 2>/dev/null || true
fi

# 辅助函数：清理指定名称的僵尸进程或强制终止
cleanup_process() {
    local name="$1"
    local force="$2"

    for pid in $(pgrep -f "$name" 2>/dev/null); do
        [ -z "$pid" ] && continue
        if [ "$force" = "1" ]; then
            kill -9 "$pid" 2>/dev/null || true
        elif [ -f "/proc/$pid/status" ]; then
            if grep -q 'State:.*Z' "/proc/$pid/status" 2>/dev/null; then
                local ppid=$(grep '^PPid:' "/proc/$pid/status" 2>/dev/null | awk '{print $2}')
                [ -n "$ppid" ] && [ "$ppid" -gt 1 ] && kill -9 "$ppid" 2>/dev/null || true
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
    done
}

# 2. 如果要求重置，先无条件彻底清理旧进程与文件
if [ "$FORCE_RESET" = "1" ]; then
    cleanup_process "alsaucm_test" 1
    cleanup_process "mavo-pcm-bridge" 1
    rm -f /run/alsaucm_test /run/djonehub-alsaucm.log /run/djonehub-voice-route.log
    sleep 0.3
else
    # 否则清理可能存在的僵尸死锁
    cleanup_process "alsaucm_test" 0
    cleanup_process "mavo-pcm-bridge" 0
fi

# 3. 确保 alsaucm_test 存活运行
need_alsaucm=0
alsaucm_pids=$(pgrep -f '/usr/bin/alsaucm_test' 2>/dev/null || true)
if [ -z "$alsaucm_pids" ]; then
    need_alsaucm=1
else
    for p in $alsaucm_pids; do
        if grep -q 'State:.*Z' "/proc/$p/status" 2>/dev/null; then
            cleanup_process "alsaucm_test" 1
            need_alsaucm=1
            break
        fi
    done
fi

if [ "$need_alsaucm" = "1" ] || [ ! -e /run/alsaucm_test ]; then
    rm -f /run/alsaucm_test /run/djonehub-alsaucm.log
    mkfifo /run/alsaucm_test 2>/dev/null || true
    chmod 666 /run/alsaucm_test 2>/dev/null || true
    nohup /usr/bin/alsaucm_test </dev/null >> /run/djonehub-alsaucm.log 2>&1 &
    sleep 0.5
fi

# 4. 执行 ACDB VoLTE 校准与路由
if ! grep -q 'ACDB -> Sent VocProc Cal!' /run/djonehub-alsaucm.log 2>/dev/null || [ "$FORCE_RESET" = "1" ]; then
    if [ -e /run/alsaucm_test ]; then
        (
            printf 'open snd_soc_msm_9x07_Tomtom_I2S\n'
            printf 'set _verb VoLTE\n'
            printf 'set _enadev Auxpcm Rx\n'
            printf 'set _enadev Auxpcm Tx\n'
        ) > /run/alsaucm_test 2>/dev/null || true
        sleep 0.5
    fi
fi

# 5. 确保 6 项高通核心 QDSP6 混音器开关全部保持开启
/usr/bin/amix 564 1 2>/dev/null || true # VoLTE_Tx Mixer SEC_AUX_PCM_TX_VoLTE
/usr/bin/amix 666 1 2>/dev/null || true # SEC_AUX_PCM_RX_Voice Mixer VoLTE
/usr/bin/amix 562 1 2>/dev/null || true # VoLTE_Tx Mixer AFE_PCM_TX_VoLTE
/usr/bin/amix 689 1 2>/dev/null || true # AFE_PCM_RX_Voice Mixer VoLTE
/usr/bin/amix 765 1 2>/dev/null || true # Incall_Music Audio Mixer MultiMedia1
/usr/bin/amix 1004 1 2>/dev/null || true # MultiMedia1 Mixer VOC_REC_DL

# 6. 确保 mavo-pcm-bridge 会话存活运行
need_mavo=0
mavo_pids=$(pgrep -f 'mavo-pcm-bridge.armv7 --voice-route-session' 2>/dev/null || true)
if [ -z "$mavo_pids" ]; then
    need_mavo=1
else
    for p in $mavo_pids; do
        if grep -q 'State:.*Z' "/proc/$p/status" 2>/dev/null; then
            cleanup_process "mavo-pcm-bridge" 1
            need_mavo=1
            break
        fi
    done
fi

if [ "$need_mavo" = "1" ]; then
    rm -f /run/djonehub-voice-route.log
    nohup /tmp/djonehub-call/mavo-pcm-bridge.armv7 --voice-route-session --verbose </dev/null >> /run/djonehub-voice-route.log 2>&1 &
    sleep 0.5
fi

echo "QDC507 Voice Route Setup Complete"
