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

// EnsureQDC507VoiceRoute 异步确保模组底层语音通路、ACDB 校准与 Mixer 全部处于激活状态
func EnsureQDC507VoiceRoute() {
	go func() {
		qdc507Mu.Lock()
		defer qdc507Mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// 检查模组中 setup_voice_route.sh 是否存在
		checkCmd := exec.CommandContext(ctx, "adb", "shell", "test -f /tmp/djonehub-call/setup_voice_route.sh")
		if err := checkCmd.Run(); err != nil {
			logger.Info("模组尚未初始化音频运行时，正在推送驱动与脚本...")
			_ = exec.CommandContext(ctx, "adb", "shell", "mkdir -p /tmp/djonehub-call && chmod 700 /tmp/djonehub-call").Run()
			_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_aprv3.ko", "/tmp/djonehub-call/").Run()
			_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/qdc507_voice.ko", "/tmp/djonehub-call/").Run()
			_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/mavo-pcm-bridge.armv7", "/tmp/djonehub-call/").Run()
			_ = exec.CommandContext(ctx, "adb", "push", "/app/module_voice/setup_voice_route.sh", "/tmp/djonehub-call/").Run()
			_ = exec.CommandContext(ctx, "adb", "shell", "chmod 755 /tmp/djonehub-call/mavo-pcm-bridge.armv7 /tmp/djonehub-call/setup_voice_route.sh").Run()
		}

		// 执行初始化与 Mixer 配置
		runCmd := exec.CommandContext(ctx, "adb", "shell", "/tmp/djonehub-call/setup_voice_route.sh")
		out, err := runCmd.CombinedOutput()
		if err != nil {
			logger.Warn("执行 setup_voice_route 出现警告", "err", err, "out", strings.TrimSpace(string(out)))
		} else {
			logger.Info("大疆 QDC507 模组硬件语音路由已激活", "msg", strings.TrimSpace(string(out)))
		}
	}()
}
