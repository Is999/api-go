package validators

import (
	"testing"

	"api/internal/config"
)

// TestValidateProductionRejectsPlaceholderJWTSecret 确保生产环境不能使用示例 JWT 密钥。
func TestValidateProductionRejectsPlaceholderJWTSecret(t *testing.T) {
	cfg := validProductionConfig()
	cfg.JwtSecret = "replace-with-strong-secret"
	if err := ValidateProduction(cfg); err == nil {
		t.Fatal("expected placeholder jwt_secret to be rejected")
	}
}

// TestValidateProductionRejectsMissingOpsToken 确保生产环境必须配置热加载运维令牌。
func TestValidateProductionRejectsMissingOpsToken(t *testing.T) {
	cfg := validProductionConfig()
	cfg.Ops.ConfigReloadToken = ""
	if err := ValidateProduction(cfg); err == nil {
		t.Fatal("expected missing ops token to be rejected")
	}
}

// TestValidateProductionRequiresCollector 确保生产认证风控事件不会被静默丢弃。
func TestValidateProductionRequiresCollector(t *testing.T) {
	cfg := validProductionConfig()
	cfg.Collector.Enabled = false
	if err := ValidateProduction(cfg); err == nil {
		t.Fatal("expected disabled collector to be rejected")
	}
}

// TestValidateProductionRequiresAuthSecurityRoute 确保生产认证事件固定进入 Admin 消费 Topic。
func TestValidateProductionRequiresAuthSecurityRoute(t *testing.T) {
	for _, topic := range []string{"", "wrong_topic"} {
		cfg := validProductionConfig()
		cfg.Collector.Tasks[config.CollectorBizTypeAuthSecurity] = config.CollectorTaskConfig{Topic: topic}
		if err := ValidateProduction(cfg); err == nil {
			t.Fatalf("expected auth.security topic %q to be rejected", topic)
		}
	}
}

// validProductionConfig 返回满足生产硬校验的最小配置。
func validProductionConfig() config.Config {
	cfg := config.Config{
		JwtSecret: "prod-jwt-9f3b6e1c7a2d4f0b8c5e6a1d2f3c4b5a",
		Auth: config.AuthConfig{
			LoginRateLimit: config.AuthRateLimitConfig{
				Enabled: true,
			},
			RegisterRateLimit: config.AuthRateLimitConfig{
				Enabled: true,
			},
		},
		Ops: config.OpsConfig{
			ConfigReloadToken: "prod-ops-9f3b6e1c7a2d4f0b",
		},
		Collector: config.CollectorConfig{
			Enabled: true,
			Tasks: map[string]config.CollectorTaskConfig{
				config.CollectorBizTypeAuthSecurity: {Topic: config.CollectorTopicAuthSecurity},
			},
		},
	}
	cfg.Mode = "pro"
	return cfg
}
