package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"api/common/runtimecfg"
	"api/internal/config"
	"api/internal/svc"
)

// TestDetectHotReloadRestartImpact 验证运行期字段热加载不会误触发重启，监听端口变化必须提示重启。
func TestDetectHotReloadRestartImpact(t *testing.T) {
	oldCfg := config.Config{
		HotReload: config.HotReloadConfig{
			Enabled:              true,
			CheckIntervalSeconds: 5,
		},
	}
	oldCfg.Mode = "dev"
	newCfg := oldCfg
	newCfg.Mode = "prod"
	newCfg.HotReload.CheckIntervalSeconds = 10

	restartRequired, reason := detectHotReloadRestartImpact(oldCfg, newCfg)
	if restartRequired || reason != "" {
		t.Fatalf("runtime config should not require restart: restart=%v reason=%q", restartRequired, reason)
	}

	newCfg.Port = oldCfg.Port + 1
	restartRequired, reason = detectHotReloadRestartImpact(oldCfg, newCfg)
	if !restartRequired || reason == "" {
		t.Fatalf("port change should require restart: restart=%v reason=%q", restartRequired, reason)
	}
}

// TestHotReloadRestartSpecsValid 确保热加载重启边界规格完整且顺序稳定。
func TestHotReloadRestartSpecsValid(t *testing.T) {
	specs := hotReloadRestartSpecs()
	wantReasons := []string{
		"HTTP监听地址变更",
		"雪花ID worker 配置变更",
		"用户写入分表路由配置变更",
		"MySQL连接配置变更",
		"Redis连接配置变更",
		"OTLP导出配置变更",
	}
	if len(specs) != len(wantReasons) {
		t.Fatalf("热加载重启边界数量不符合预期: got=%d want=%d", len(specs), len(wantReasons))
	}
	seen := make(map[string]struct{}, len(specs))
	for index, spec := range specs {
		if spec.Reason != wantReasons[index] {
			t.Fatalf("热加载重启边界顺序不符合预期: index=%d got=%s want=%s", index, spec.Reason, wantReasons[index])
		}
		if spec.Changed == nil {
			t.Fatalf("热加载重启边界缺少变化判断: %s", spec.Reason)
		}
		if spec.Preserve == nil {
			t.Fatalf("热加载重启边界缺少原值保留逻辑: %s", spec.Reason)
		}
		if _, ok := seen[spec.Reason]; ok {
			t.Fatalf("热加载重启边界重复: %s", spec.Reason)
		}
		seen[spec.Reason] = struct{}{}
	}
}

// TestBuildHotReloadEffectiveConfigPreservesRestartOnlyFields 确保待重启字段保留原值，运行期字段仍可刷新。
func TestBuildHotReloadEffectiveConfigPreservesRestartOnlyFields(t *testing.T) {
	oldCfg := config.Config{
		AppID: "old-app",
		MySQL: config.MySQLConfig{
			WriteDataSource: "old-write",
			MaxOpenConns:    10,
		},
		SiteMySQL: config.SiteMySQLConfig{
			"site": {WriteDataSource: "old-site"},
		},
		Redis: config.RedisConfig{
			Addrs: []string{"127.0.0.1:6379"},
		},
		Snowflake: config.SnowflakeConfig{
			WorkerID: int64Ptr(1),
		},
		User: config.UserConfig{
			RouteShardCount: 1,
		},
		Observability: config.ObservabilityConfig{
			ServiceName:  "old-service",
			OTLPEndpoint: "old-collector:4317",
			OTLPProtocol: "grpc",
		},
	}
	oldCfg.Host = "127.0.0.1"
	oldCfg.Port = 8890
	oldCfg.Mode = "dev"

	newCfg := oldCfg
	newCfg.AppID = "new-app"
	newCfg.Host = "0.0.0.0"
	newCfg.Port = 8891
	newCfg.Mode = "prod"
	newCfg.Snowflake.WorkerID = int64Ptr(2)
	newCfg.User.RouteShardCount = 10
	newCfg.MySQL = config.MySQLConfig{WriteDataSource: "new-write", MaxOpenConns: 20}
	newCfg.SiteMySQL = config.SiteMySQLConfig{"site": {WriteDataSource: "new-site"}}
	newCfg.Redis = config.RedisConfig{Addrs: []string{"127.0.0.1:6380"}}
	newCfg.Observability.ServiceName = "new-service"
	newCfg.Observability.OTLPEndpoint = "new-collector:4317"
	newCfg.Observability.OTLPProtocol = "http"

	effective := buildHotReloadEffectiveConfig(oldCfg, newCfg)
	if effective.Host != oldCfg.Host || effective.Port != oldCfg.Port || effective.Mode != oldCfg.Mode {
		t.Fatalf("期望 HTTP 服务配置保持原值，实际 host=%s port=%d mode=%s", effective.Host, effective.Port, effective.Mode)
	}
	if effective.MySQL.WriteDataSource != oldCfg.MySQL.WriteDataSource {
		t.Fatalf("期望 MySQL 保持原值，实际为 %+v", effective.MySQL)
	}
	if effective.SiteMySQL["site"].WriteDataSource != oldCfg.SiteMySQL["site"].WriteDataSource {
		t.Fatalf("期望 SiteMySQL 保持原值，实际为 %+v", effective.SiteMySQL)
	}
	if effective.Redis.Addrs[0] != oldCfg.Redis.Addrs[0] {
		t.Fatalf("期望 Redis 保持原值，实际为 %+v", effective.Redis)
	}
	if effective.Snowflake.WorkerID == nil || *effective.Snowflake.WorkerID != *oldCfg.Snowflake.WorkerID {
		t.Fatalf("期望雪花 worker_id 保持原值，实际为 %+v", effective.Snowflake)
	}
	if effective.User.RouteShardCount != oldCfg.User.RouteShardCount {
		t.Fatalf("期望用户写入分表路由保持原值，实际为 %+v", effective.User)
	}
	if effective.Observability.OTLPEndpoint != oldCfg.Observability.OTLPEndpoint ||
		effective.Observability.OTLPProtocol != oldCfg.Observability.OTLPProtocol {
		t.Fatalf("期望 OTLP 导出配置保持原值，实际为 %+v", effective.Observability)
	}
	if effective.Observability.ServiceName != newCfg.Observability.ServiceName {
		t.Fatalf("期望观测运行参数刷新为新值，实际 service_name=%s", effective.Observability.ServiceName)
	}
	if effective.AppID != newCfg.AppID {
		t.Fatalf("期望普通运行期配置刷新为新值，实际 app_id=%s", effective.AppID)
	}
}

// TestNormalizeHotReloadCheckInterval 验证热加载轮询间隔默认值和显式配置值。
func TestNormalizeHotReloadCheckInterval(t *testing.T) {
	if got := normalizeHotReloadCheckInterval(0); got != 5*time.Second {
		t.Fatalf("interval 0 = %s, want 5s", got)
	}
	if got := normalizeHotReloadCheckInterval(2); got != 2*time.Second {
		t.Fatalf("interval 2 = %s, want 2s", got)
	}
}

// TestReloadConfigFileSkipsUnchangedSnapshot 验证配置文件未变化时不会重复发布运行时快照。
func TestReloadConfigFileSkipsUnchangedSnapshot(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
Name: "api"
Host: "127.0.0.1"
Port: 8890
Mode: "dev"
app_id: "1"
snowflake:
  worker_id: 1
jwt_secret: "test-secret-please-change"
auth:
  password_min_length: 8
hot_reload:
  enabled: false
redis:
  addrs:
    - "127.0.0.1:6379"
  password: ""
  db: 0
  pool_size: 1
`), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	cfg, version, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	svcCtx := svc.NewServiceContext(cfg, version, svc.Dependencies{})
	svcCtx.UpdateHotReloadStatus(svc.HotReloadStatus{
		ConfigVersion: version,
		ReloadCount:   3,
	})
	app := &App{ServiceContext: svcCtx}

	prev := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: "stable-app"})
	t.Cleanup(func() {
		runtimecfg.Restore(prev)
	})
	if _, err = app.reloadConfigFile(context.Background(), "manual_api", configFile); err != nil {
		t.Fatalf("reloadConfigFile() error = %v", err)
	}

	status := svcCtx.CurrentHotReloadStatus()
	if status.ReloadCount != 3 {
		t.Fatalf("配置无变化不应增加 ReloadCount，实际为 %d", status.ReloadCount)
	}
	if status.LastMessage != "配置无变化" {
		t.Fatalf("期望记录配置无变化，实际为 %q", status.LastMessage)
	}
	if got := runtimecfg.AppID(); got != "stable-app" {
		t.Fatalf("配置无变化不应重复设置 runtimecfg，实际 app_id=%q", got)
	}
}
