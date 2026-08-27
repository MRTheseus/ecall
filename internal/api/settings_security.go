package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/pkg/logger"
)

type securitySettingsResponse struct {
	RateLimit struct {
		Enabled       bool `json:"enabled"`
		MaxAttempts   int  `json:"max_attempts"`
		WindowSeconds int  `json:"window_seconds"`
	} `json:"rate_limit"`
	AntiIPSpoofing bool     `json:"anti_ip_spoofing"`
	TrustedProxies []string `json:"trusted_proxies"`
}

type updateSecuritySettingsRequest struct {
	RateLimit struct {
		Enabled       bool `json:"enabled"`
		MaxAttempts   int  `json:"max_attempts"`
		WindowSeconds int  `json:"window_seconds"`
	} `json:"rate_limit"`
	AntiIPSpoofing bool     `json:"anti_ip_spoofing"`
	TrustedProxies []string `json:"trusted_proxies"`
}

// handleGetSecuritySettings 获取系统安全防护与限频设置
func (s *Server) handleGetSecuritySettings(c *gin.Context) {
	sec := s.getSecurityConfig()

	resp := securitySettingsResponse{
		RateLimit: struct {
			Enabled       bool `json:"enabled"`
			MaxAttempts   int  `json:"max_attempts"`
			WindowSeconds int  `json:"window_seconds"`
		}{
			Enabled:       sec.RateLimit.Enabled,
			MaxAttempts:   sec.RateLimit.MaxAttempts,
			WindowSeconds: sec.RateLimit.WindowSeconds,
		},
		AntiIPSpoofing: sec.AntiIPSpoofing,
		TrustedProxies: sec.TrustedProxies,
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"security": resp,
	})
}

// handleUpdateSecuritySettings 更新并持久化系统安全防护与限频设置
func (s *Server) handleUpdateSecuritySettings(c *gin.Context) {
	var req updateSecuritySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "参数格式错误: " + err.Error(),
		})
		return
	}

	if req.RateLimit.Enabled {
		if req.RateLimit.MaxAttempts <= 0 || req.RateLimit.MaxAttempts > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "登录尝试限制次数必须在 1 ~ 100 之间",
			})
			return
		}
		if req.RateLimit.WindowSeconds < 10 || req.RateLimit.WindowSeconds > 3600 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "限频时间窗口必须在 10 ~ 3600 秒之间",
			})
			return
		}
	} else {
		if req.RateLimit.MaxAttempts <= 0 {
			req.RateLimit.MaxAttempts = 10
		}
		if req.RateLimit.WindowSeconds <= 0 {
			req.RateLimit.WindowSeconds = 120
		}
	}

	trustedProxies := make([]string, 0, len(req.TrustedProxies))
	for _, p := range req.TrustedProxies {
		if p != "" {
			trustedProxies = append(trustedProxies, p)
		}
	}

	secCfg := config.SecurityConfig{
		RateLimit: config.RateLimitConfig{
			Enabled:       req.RateLimit.Enabled,
			MaxAttempts:   req.RateLimit.MaxAttempts,
			WindowSeconds: req.RateLimit.WindowSeconds,
		},
		AntiIPSpoofing: req.AntiIPSpoofing,
		TrustedProxies: trustedProxies,
	}

	if err := config.UpdateSecurityInFile(s.configPath, secCfg); err != nil {
		logger.Error("写入安全配置失败", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "写入配置文件失败: " + err.Error(),
		})
		return
	}

	s.applySecurityConfig(secCfg)
	logger.Info("安全防护与限频设置已更新",
		"rate_limit_enabled", secCfg.RateLimit.Enabled,
		"max_attempts", secCfg.RateLimit.MaxAttempts,
		"window_seconds", secCfg.RateLimit.WindowSeconds,
		"anti_ip_spoofing", secCfg.AntiIPSpoofing,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"security": secCfg,
		"message":  "安全配置已成功保存",
	})
}
