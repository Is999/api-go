package bootstrap

import (
	"testing"
	"time"

	"api/internal/config"
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
