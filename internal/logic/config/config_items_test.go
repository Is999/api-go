package config

import (
	"math"
	"strings"
	"testing"

	appconfig "api/internal/config"
	"api/internal/types"
)

// TestBuildMaskedConfigViewMasksSensitiveValues 验证对应场景符合预期。
func TestBuildMaskedConfigViewMasksSensitiveValues(t *testing.T) {
	cfg := appconfig.Config{
		AppID:        "site-a",
		AppKey:       "app-secret-value",
		JwtSecret:    "jwt-secret-value",
		JwtExpiresIn: 3600,
		Redis: appconfig.RedisConfig{
			Addrs:    []string{"127.0.0.1:6379"},
			Password: "redis-password",
			PoolSize: 8,
		},
		Ops: appconfig.OpsConfig{
			ConfigReloadToken: "ops-token-value",
		},
	}
	view, err := buildMaskedConfigView(cfg)
	if err != nil {
		t.Fatalf("buildMaskedConfigView() error = %v", err)
	}
	snapshot := view.snapshotYAML
	for _, secret := range []string{"app-secret-value", "jwt-secret-value", "redis-password", "ops-token-value", "127.0.0.1:6379"} {
		if strings.Contains(snapshot, secret) {
			t.Fatalf("脱敏快照泄露敏感值 %q: %s", secret, snapshot)
		}
	}
	if view.sensitiveTotal == 0 {
		t.Fatal("期望至少识别出一个敏感配置项")
	}
}

// TestPaginateConfigItemsRejectsOverflowingPage 校验极大页码不会整数溢出后回读首批数据。
func TestPaginateConfigItemsRejectsOverflowingPage(t *testing.T) {
	items := []types.ConfigItem{{Path: "app_id"}}
	got := paginateConfigItems(items, math.MaxInt, 100)
	if len(got) != 0 {
		t.Fatalf("期望极大页码返回空列表，实际 %#v", got)
	}
}

// TestBuildMaskedRuntimeYAMLOnlyIncludesRuntimeSections 验证对应场景符合预期。
func TestBuildMaskedRuntimeYAMLOnlyIncludesRuntimeSections(t *testing.T) {
	cfg := appconfig.Config{
		JwtSecret: "jwt-secret-value",
		Auth: appconfig.AuthConfig{
			RegisterEnabled: true,
		},
		HotReload: appconfig.HotReloadConfig{
			Enabled:              true,
			CheckIntervalSeconds: 5,
		},
		MySQL: appconfig.MySQLConfig{
			WriteDataSource: "user:pass@tcp(127.0.0.1:3306)/api",
		},
	}
	yamlText, err := buildMaskedRuntimeYAML(cfg)
	if err != nil {
		t.Fatalf("buildMaskedRuntimeYAML() error = %v", err)
	}
	if !strings.Contains(yamlText, "auth:") || !strings.Contains(yamlText, "hot_reload:") {
		t.Fatalf("运行期 YAML 缺少外置配置段: %s", yamlText)
	}
	if strings.Contains(yamlText, "mysql:") || strings.Contains(yamlText, "jwt_secret") {
		t.Fatalf("运行期 YAML 不应包含启动期配置段: %s", yamlText)
	}
}
