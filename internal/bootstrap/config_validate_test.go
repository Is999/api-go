package bootstrap

import (
	"testing"

	"api/internal/config"
)

// TestValidateConfigRejectsWeakJWTSecret 确保明显弱 JWT 密钥不能通过启动校验。
func TestValidateConfigRejectsWeakJWTSecret(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.JwtSecret = "short"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected weak jwt_secret to be rejected")
	}
}

// TestValidateConfigRejectsInvalidCollectorRedis 确保强制 Redis 载体时必须配置 Stream。
func TestValidateConfigRejectsInvalidCollectorRedis(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Collector = config.CollectorConfig{
		Enabled:   true,
		Transport: "redis",
		Redis: config.CollectorRedisConfig{
			Enabled: true,
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected redis collector without stream to be rejected")
	}
}

// TestValidateConfigRejectsMissingAppID 确保 app_id 缺失时不会落到共享 Redis 默认命名空间。
func TestValidateConfigRejectsMissingAppID(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = ""
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected missing app_id to be rejected")
	}
}

// TestValidateConfigRejectsCollectorRedisEnabledWithoutStream 确保启用 Redis Stream 载体时必须配置 Stream。
func TestValidateConfigRejectsCollectorRedisEnabledWithoutStream(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Collector.Redis.Enabled = true
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected collector.redis.enabled without stream to be rejected")
	}
}

// TestValidateConfigRejectsForeignCollectorStream 确保 Collector 不会误用其它站点 Redis Stream。
func TestValidateConfigRejectsForeignCollectorStream(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "site-2"
	cfg.Collector.Redis.Stream = "app:site-1:collector:events"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected foreign collector.redis.stream to be rejected")
	}
}

// TestValidateConfigRejectsPublicOpsAllowedIP 确保运维白名单不能误配公网 IP。
func TestValidateConfigRejectsPublicOpsAllowedIP(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Ops.ConfigReloadAllowedIPs = []string{"8.8.8.8"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected public ops allowed IP to be rejected")
	}
}

// TestValidateConfigAcceptsPrivateOpsCIDR 确保内网 CIDR 白名单配置可启动。
func TestValidateConfigAcceptsPrivateOpsCIDR(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Ops.ConfigReloadAllowedIPs = []string{"10.0.0.0/8", "127.0.0.1"}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
}

// TestValidateConfigRejectsLargeAuthRateLimit 确保极端限流窗口不能通过启动校验。
func TestValidateConfigRejectsLargeAuthRateLimit(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Auth.LoginRateLimit = config.AuthRateLimitConfig{
		Enabled:       true,
		WindowSeconds: maxAuthRateLimitWindowSeconds + 1,
		MaxAttempts:   5,
		LockSeconds:   300,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected oversized auth rate limit window to be rejected")
	}
}

// TestValidateConfigRejectsProductionPlaceholderJWTSecret 确保生产环境不能使用示例 JWT 密钥。
func TestValidateConfigRejectsProductionPlaceholderJWTSecret(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.JwtSecret = "replace-with-strong-secret"
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected production placeholder jwt_secret to be rejected")
	}
}

// TestValidateConfigRejectsProductionMissingOpsToken 确保生产环境必须配置热加载运维令牌。
func TestValidateConfigRejectsProductionMissingOpsToken(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Ops.ConfigReloadToken = ""
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected missing production ops token to be rejected")
	}
}

// TestValidateConfigRejectsProductionRedisTLSInsecure 确保生产环境不能跳过 Redis TLS 校验。
func TestValidateConfigRejectsProductionRedisTLSInsecure(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Redis.TLSInsecureSkipVerify = true
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected production redis tls insecure skip verify to be rejected")
	}
}

// TestValidateConfigRejectsProductionDisabledLoginRateLimit 确保生产环境必须启用登录限流。
func TestValidateConfigRejectsProductionDisabledLoginRateLimit(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Auth.LoginRateLimit.Enabled = false
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected production disabled login rate limit to be rejected")
	}
}

// TestValidateConfigRejectsProductionRegisterWithoutRateLimit 确保生产开放注册时必须启用注册限流。
func TestValidateConfigRejectsProductionRegisterWithoutRateLimit(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Auth.RegisterEnabled = true
	cfg.Auth.RegisterRateLimit.Enabled = false
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected production register without rate limit to be rejected")
	}
}

// TestValidateConfigAcceptsProductionSafeConfig 确保生产安全配置可以通过启动校验。
func TestValidateConfigAcceptsProductionSafeConfig(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
}

func validBootstrapConfig() config.Config {
	return config.Config{
		AppID:     "1",
		JwtSecret: "test-secret-please-change",
		Auth: config.AuthConfig{
			PasswordMinLength: 8,
		},
		Redis: config.RedisConfig{
			Addrs:    []string{"127.0.0.1:6379"},
			PoolSize: 1,
		},
	}
}

func validProductionBootstrapConfig() config.Config {
	cfg := validBootstrapConfig()
	cfg.Mode = "pro"
	cfg.JwtSecret = "prod-jwt-9f3b6e1c7a2d4f0b8c5e6a1d2f3c4b5a"
	cfg.Auth.LoginRateLimit = config.AuthRateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxAttempts:   5,
		LockSeconds:   300,
	}
	cfg.Auth.RegisterRateLimit = config.AuthRateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxAttempts:   3,
		LockSeconds:   600,
	}
	cfg.Ops.ConfigReloadToken = "prod-ops-9f3b6e1c7a2d4f0b"
	return cfg
}
