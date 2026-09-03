package voicecall

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
)

var (
	qdc507Mu sync.Mutex
)

// ResetQDC507VoiceRoute 服务冷启动时，强制清场模组底座残留僵尸并全新激活音频路由
func ResetQDC507VoiceRoute() {
	go func() {
		qdc507Mu.Lock()
		defer qdc507Mu.Unlock()

		runQDC507Setup(true)
	}()
}

// EnsureQDC507VoiceRoute 同步确保模组底层语音通路、ACDB 校准与 Mixer 全部处于激活状态（呼叫前快速自愈）
func EnsureQDC507VoiceRoute() {
	qdc507Mu.Lock()
	defer qdc507Mu.Unlock()

	runQDC507Setup(false)
}

func runQDC507Setup(forceReset bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_ = exec.CommandContext(ctx, "adb", "shell", "mkdir -p /tmp/djonehub-call && chmod 700 /tmp/djonehub-call").Run()
	_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/setup_voice_route.sh", "/tmp/djonehub-call/").Run()
	_ = exec.CommandContext(ctx, "adb", "shell", "chmod 755 /tmp/djonehub-call/setup_voice_route.sh").Run()

	if forceReset {
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_aprv3.ko", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_voice.ko", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/mavo-pcm-bridge.armv7", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "shell", "chmod 755 /tmp/djonehub-call/mavo-pcm-bridge.armv7").Run()
	}

	// 执行初始化与 Mixer 配置
	args := []string{"shell", "/tmp/djonehub-call/setup_voice_route.sh"}
	if forceReset {
		args = append(args, "--reset")
	}
	runCmd := exec.CommandContext(ctx, "adb", args...)
	out, err := runCmd.CombinedOutput()
	if err != nil {
		logger.Warn("执行 setup_voice_route 出现警告", "err", err, "out", strings.TrimSpace(string(out)))
	} else {
		logger.Info("大疆 QDC507 模组硬件语音路由已激活", "msg", strings.TrimSpace(string(out)), "reset", forceReset)
	}
}
