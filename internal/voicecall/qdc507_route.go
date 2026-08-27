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

// EnsureQDC507VoiceRoute 异步确保模组底层语音通路、ACDB 校准与 Mixer 全部处于激活状态（呼叫前快速自愈）
func EnsureQDC507VoiceRoute() {
	go func() {
		qdc507Mu.Lock()
		defer qdc507Mu.Unlock()

		runQDC507Setup(false)
	}()
}

func runQDC507Setup(forceReset bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 检查模组中 setup_voice_route.sh 是否存在或是否需要强制覆盖同步
	checkCmd := exec.CommandContext(ctx, "adb", "shell", "test -f /tmp/djonehub-call/setup_voice_route.sh")
	needPush := false
	if err := checkCmd.Run(); err != nil || forceReset {
		needPush = true
	}

	if needPush {
		_ = exec.CommandContext(ctx, "adb", "shell", "mkdir -p /tmp/djonehub-call && chmod 700 /tmp/djonehub-call").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_aprv3.ko", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_voice.ko", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/mavo-pcm-bridge.armv7", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/setup_voice_route.sh", "/tmp/djonehub-call/").Run()
		_ = exec.CommandContext(ctx, "adb", "shell", "chmod 755 /tmp/djonehub-call/mavo-pcm-bridge.armv7 /tmp/djonehub-call/setup_voice_route.sh").Run()
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
