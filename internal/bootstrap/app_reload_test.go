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

func TestNormalizeHotReloadCheckInterval(t *testing.T) {
	if got := normalizeHotReloadCheckInterval(0); got != 5*time.Second {
		t.Fatalf("interval 0 = %s, want 5s", got)
	}
	if got := normalizeHotReloadCheckInterval(2); got != 2*time.Second {
		t.Fatalf("interval 2 = %s, want 2s", got)
	}
}

func TestReloadConfigFileSkipsUnchangedSnapshot(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(`
Name: "api"
Host: "127.0.0.1"
Port: 8890
Mode: "dev"
app_id: "1"
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
