package validators

import (
	"testing"

	"api/internal/config"
)

// TestValidateCollectorRejectsRedisEnabledWithoutStream 确保启用 Redis Stream 载体时必须配置 Stream。
func TestValidateCollectorRejectsRedisEnabledWithoutStream(t *testing.T) {
	cfg := config.Config{
		AppID: "site-1",
		Collector: config.CollectorConfig{
			Redis: config.CollectorRedisConfig{Enabled: true},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected collector.redis.enabled without stream to be rejected")
	}
}

// TestValidateCollectorRejectsForeignStream 确保 Collector 不会误用其它站点 Redis Stream。
func TestValidateCollectorRejectsForeignStream(t *testing.T) {
	cfg := config.Config{
		AppID: "site-2",
		Collector: config.CollectorConfig{
			Redis: config.CollectorRedisConfig{Stream: "app:site-1:collector:events"},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected foreign collector stream to be rejected")
	}
}
