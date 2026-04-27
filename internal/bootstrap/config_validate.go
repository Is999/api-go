package bootstrap

import (
	"net/netip"
	"strings"

	keys "api/common/rediskeys"
	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

// 启动配置校验边界常量。
const (
	minJWTSecretLength            = 16    // JWT 密钥最小长度，避免明显弱配置启动
	minOpsTokenLength             = 16    // 运维令牌生产环境最小长度
	minPasswordLength             = 6     // 前台密码最小允许长度下限
	maxAuthRateLimitWindowSeconds = 3600  // 认证限流最大统计窗口
	maxAuthRateLimitLockSeconds   = 86400 // 认证限流最大锁定时长
	maxAuthRateLimitAttempts      = 1000  // 认证限流最大尝试次数
	collectorTransportAuto        = "auto"
	collectorTransportRedis       = "redis"
	collectorTransportSync        = "sync"
)

// validateConfig 校验启动必填配置，避免服务以明显错误状态启动。
func validateConfig(c config.Config) error {
	if len(strings.TrimSpace(c.JwtSecret)) < minJWTSecretLength {
		return errors.Errorf("jwt_secret 长度不能小于 %d", minJWTSecretLength)
	}
	if strings.TrimSpace(c.AppID) == "" {
		return errors.Errorf("app_id 不能为空")
	}
	if len(c.Redis.Addrs) == 0 {
		return errors.Errorf("redis.addrs 不能为空")
	}
	if c.Redis.PoolSize <= 0 {
		return errors.Errorf("redis.pool_size 必须大于 0")
	}
	if c.Auth.PasswordMinLength < minPasswordLength {
		return errors.Errorf("auth.password_min_length 不能小于 %d", minPasswordLength)
	}
	if err := validateAuthRateLimitConfig("auth.login_rate_limit", c.Auth.LoginRateLimit); err != nil {
		return errors.Tag(err)
	}
	if err := validateAuthRateLimitConfig("auth.register_rate_limit", c.Auth.RegisterRateLimit); err != nil {
		return errors.Tag(err)
	}
	if err := validateCollectorConfig(c); err != nil {
		return errors.Tag(err)
	}
	if err := validateOpsConfig(c.Ops); err != nil {
		return errors.Tag(err)
	}
	if err := validateSecurityConfig(c); err != nil {
		return errors.Tag(err)
	}
	if err := validateProductionConfig(c); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateAuthRateLimitConfig 校验认证限流参数是否在可控范围内。
func validateAuthRateLimitConfig(name string, cfg config.AuthRateLimitConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.WindowSeconds > maxAuthRateLimitWindowSeconds {
		return errors.Errorf("%s.window_seconds 不能大于 %d", name, maxAuthRateLimitWindowSeconds)
	}
	if cfg.MaxAttempts > maxAuthRateLimitAttempts {
		return errors.Errorf("%s.max_attempts 不能大于 %d", name, maxAuthRateLimitAttempts)
	}
	if cfg.LockSeconds > maxAuthRateLimitLockSeconds {
		return errors.Errorf("%s.lock_seconds 不能大于 %d", name, maxAuthRateLimitLockSeconds)
	}
	return nil
}

// validateCollectorConfig 校验 Collector 载体配置是否自洽。
func validateCollectorConfig(c config.Config) error {
	cfg := c.Collector
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if cfg.Redis.Enabled && strings.TrimSpace(cfg.Redis.Stream) == "" {
		return errors.Errorf("collector.redis.enabled=true 时必须配置 collector.redis.stream")
	}
	if keys.IsForeignAppScopedKey(c.AppID, cfg.Redis.Stream) {
		ownerAppID, _ := keys.AppScopedAppID(cfg.Redis.Stream)
		return errors.Errorf("collector.redis.stream 属于其它 app_id[%s]", ownerAppID)
	}
	switch transport {
	case "", collectorTransportAuto, collectorTransportSync:
		return nil
	case collectorTransportRedis:
		if !cfg.Redis.Enabled {
			return errors.Errorf("collector.transport=redis 时必须启用 collector.redis.enabled")
		}
		if strings.TrimSpace(cfg.Redis.Stream) == "" {
			return errors.Errorf("collector.transport=redis 时必须配置 collector.redis.stream")
		}
		return nil
	default:
		return errors.Errorf("collector.transport 仅支持 auto/sync/redis")
	}
}

// validateOpsConfig 校验运维白名单，防止配置层误放行公网来源。
func validateOpsConfig(cfg config.OpsConfig) error {
	for _, item := range cfg.ConfigReloadAllowedIPs {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				return errors.Wrapf(err, "ops.config_reload_allowed_ips CIDR 非法: %s", item)
			}
			if !isInternalConfigAddr(prefix.Addr()) {
				return errors.Errorf("ops.config_reload_allowed_ips 不能配置公网 CIDR: %s", item)
			}
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return errors.Wrapf(err, "ops.config_reload_allowed_ips IP 非法: %s", item)
		}
		if !isInternalConfigAddr(addr) {
			return errors.Errorf("ops.config_reload_allowed_ips 不能配置公网 IP: %s", item)
		}
	}
	return nil
}

// validateProductionConfig 校验生产环境禁止使用的占位和不安全配置。
func validateProductionConfig(c config.Config) error {
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

// isInternalConfigAddr 判断配置中的地址是否属于内网、本机或链路本地。
func isInternalConfigAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}
