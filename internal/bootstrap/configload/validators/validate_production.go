package validators

import (
	"strings"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

const minOpsTokenLength = 16 // 运维令牌生产环境最小长度

// ValidateProduction 校验生产环境禁止使用的占位和不安全配置。
func ValidateProduction(c config.Config) error {
	if !isProductionMode(c.Mode) {
		return nil
	}
	if isPlaceholderSecret(c.JwtSecret) {
		return errors.Errorf("生产环境 jwt_secret 不能使用占位值")
	}
	if c.Redis.TLSInsecureSkipVerify {
		return errors.Errorf("生产环境 redis.tls_insecure_skip_verify 不能为 true")
	}
	if !c.Auth.LoginRateLimit.Enabled {
		return errors.Errorf("生产环境必须启用 auth.login_rate_limit")
	}
	if c.Auth.RegisterEnabled && !c.Auth.RegisterRateLimit.Enabled {
		return errors.Errorf("生产环境开放注册时必须启用 auth.register_rate_limit")
	}
	token := strings.TrimSpace(c.Ops.ConfigReloadToken)
	if len(token) < minOpsTokenLength {
		return errors.Errorf("生产环境 ops.config_reload_token 长度不能小于 %d", minOpsTokenLength)
	}
	if isPlaceholderSecret(token) {
		return errors.Errorf("生产环境 ops.config_reload_token 不能使用占位值")
	}
	return nil
}

// isProductionMode 判断当前配置是否为生产运行模式。
func isProductionMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "pro", "prod", "production":
		return true
	default:
		return false
	}
}

// isPlaceholderSecret 判断密钥是否仍为示例占位值。
func isPlaceholderSecret(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, pattern := range []string{"replace-with", "please-change", "change-me", "changeme", "your-", "todo"} {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}
