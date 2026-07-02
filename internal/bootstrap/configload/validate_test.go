package configload

import (
	"testing"

	"api/internal/config"
)

// TestValidateConfigRejectsWeakJWTSecret 确保明显弱 JWT 密钥不能通过启动校验。
func TestValidateConfigRejectsWeakJWTSecret(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.JwtSecret = "short"
	if err := Validate(cfg); err == nil {
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
	if err := Validate(cfg); err == nil {
		t.Fatal("expected redis collector without stream to be rejected")
	}
}

// TestValidateConfigRejectsMissingAppID 确保 app_id 缺失时不会落到共享 Redis 默认命名空间。
func TestValidateConfigRejectsMissingAppID(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = ""
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing app_id to be rejected")
	}
}

// TestValidateConfigRejectsMissingSnowflakeWorkerID 确保雪花 worker_id 缺失时启动失败。
func TestValidateConfigRejectsMissingSnowflakeWorkerID(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Snowflake.WorkerID = nil
	t.Setenv("SNOWFLAKE_WORKER_ID", "")
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing snowflake.worker_id to be rejected")
	}
}

// TestValidateConfigRejectsInvalidSnowflakeWorkerID 确保雪花 worker_id 越界时启动失败。
func TestValidateConfigRejectsInvalidSnowflakeWorkerID(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Snowflake.WorkerID = int64Ptr(1024)
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid snowflake.worker_id to be rejected")
	}
}

// TestValidateConfigRejectsInvalidUserRouteShardCount 确保业务用户写入路由只允许平滑拆分档位。
func TestValidateConfigRejectsInvalidUserRouteShardCount(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.User.RouteShardCount = 3
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid user.route_shard_count to be rejected")
	}
}

// TestNormalizeConfigDefaultsUserRouteShardCount 确保业务用户写入路由缺省时稳定回落单表。
func TestNormalizeConfigDefaultsUserRouteShardCount(t *testing.T) {
	cfg := config.Config{}
	Normalize(&cfg)
	if cfg.User.RouteShardCount != defaultUserRouteShardCount {
		t.Fatalf("route_shard_count = %d, want %d", cfg.User.RouteShardCount, defaultUserRouteShardCount)
	}
}

// TestValidateConfigRejectsCollectorRedisEnabledWithoutStream 确保启用 Redis Stream 载体时必须配置 Stream。
func TestValidateConfigRejectsCollectorRedisEnabledWithoutStream(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Collector.Redis.Enabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected collector.redis.enabled without stream to be rejected")
	}
}

// TestValidateConfigRejectsForeignCollectorStream 确保 Collector 不会误用其它站点 Redis Stream。
func TestValidateConfigRejectsForeignCollectorStream(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "site-2"
	cfg.Collector.Redis.Stream = "app:site-1:collector:events"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected foreign collector.redis.stream to be rejected")
	}
}

// TestValidateConfigRejectsPublicOpsAllowedIP 确保运维白名单不能误配公网 IP。
func TestValidateConfigRejectsPublicOpsAllowedIP(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Ops.ConfigReloadAllowedIPs = []string{"8.8.8.8"}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected public ops allowed IP to be rejected")
	}
}

// TestValidateConfigRejectsLarkWithoutWebhook 确保启用 Lark 告警时必须配置发送端点。
func TestValidateConfigRejectsLarkWithoutWebhook(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Alert.Lark.Enabled = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected lark alert without webhook to be rejected")
	}
}

// TestValidateConfigRejectsNegativeLarkOptions 确保 Lark 数值配置不能使用负数。
func TestValidateConfigRejectsNegativeLarkOptions(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Alert.Lark.Enabled = true
	cfg.Alert.Lark.WebhookURL = "https://open.larksuite.com/open-apis/bot/v2/hook/test"
	cfg.Alert.Lark.TimeoutSeconds = -1
	if err := Validate(cfg); err == nil {
		t.Fatal("expected negative lark timeout to be rejected")
	}
	cfg.Alert.Lark.TimeoutSeconds = 1
	cfg.Alert.Lark.MaxErrorBytes = -1
	if err := Validate(cfg); err == nil {
		t.Fatal("expected negative lark max_error_bytes to be rejected")
	}
}

// TestValidateConfigAcceptsPrivateOpsCIDR 确保内网 CIDR 白名单配置可启动。
func TestValidateConfigAcceptsPrivateOpsCIDR(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Ops.ConfigReloadAllowedIPs = []string{"10.0.0.0/8", "127.0.0.1"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
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
	if err := Validate(cfg); err == nil {
		t.Fatal("expected oversized auth rate limit window to be rejected")
	}
}

// TestValidateConfigRejectsProductionPlaceholderJWTSecret 确保生产环境不能使用示例 JWT 密钥。
func TestValidateConfigRejectsProductionPlaceholderJWTSecret(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.JwtSecret = "replace-with-strong-secret"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected production placeholder jwt_secret to be rejected")
	}
}

// TestValidateConfigRejectsProductionMissingOpsToken 确保生产环境必须配置热加载运维令牌。
func TestValidateConfigRejectsProductionMissingOpsToken(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Ops.ConfigReloadToken = ""
	if err := Validate(cfg); err == nil {
		t.Fatal("expected missing production ops token to be rejected")
	}
}

// TestValidateConfigRejectsProductionRedisTLSInsecure 确保生产环境不能跳过 Redis TLS 校验。
func TestValidateConfigRejectsProductionRedisTLSInsecure(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Redis.TLSInsecureSkipVerify = true
	if err := Validate(cfg); err == nil {
		t.Fatal("expected production redis tls insecure skip verify to be rejected")
	}
}

// TestValidateConfigRejectsProductionDisabledLoginRateLimit 确保生产环境必须启用登录限流。
func TestValidateConfigRejectsProductionDisabledLoginRateLimit(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Auth.LoginRateLimit.Enabled = false
	if err := Validate(cfg); err == nil {
		t.Fatal("expected production disabled login rate limit to be rejected")
	}
}

// TestValidateConfigRejectsProductionRegisterWithoutRateLimit 确保生产开放注册时必须启用注册限流。
func TestValidateConfigRejectsProductionRegisterWithoutRateLimit(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.Auth.RegisterEnabled = true
	cfg.Auth.RegisterRateLimit.Enabled = false
	if err := Validate(cfg); err == nil {
		t.Fatal("expected production register without rate limit to be rejected")
	}
}

// TestValidateConfigAcceptsProductionSafeConfig 确保生产安全配置可以通过启动校验。
func TestValidateConfigAcceptsProductionSafeConfig(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// validBootstrapConfig 返回满足默认启动校验的 API 测试配置。
func validBootstrapConfig() config.Config {
	return config.Config{
		AppID:     "1",
		JwtSecret: "test-secret-please-change",
		Snowflake: config.SnowflakeConfig{
			WorkerID: int64Ptr(1),
		},
		Auth: config.AuthConfig{
			PasswordMinLength: 8,
		},
		Redis: config.RedisConfig{
			Addrs:    []string{"127.0.0.1:6379"},
			PoolSize: 1,
		},
	}
}

// validProductionBootstrapConfig 返回满足生产模式校验的 API 测试配置。
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

// int64Ptr 返回 int64 指针，便于构造可选配置。
func int64Ptr(value int64) *int64 {
	return &value
}
