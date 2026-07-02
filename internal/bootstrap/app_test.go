package bootstrap

import (
	"context"
	"testing"

	"api/common/runtimecfg"
	bootstrapresources "api/internal/bootstrap/resources"
	"api/internal/config"
)

// TestBuildServiceContextDoesNotPublishRuntimeConfigOnFailure 确保启动失败不会污染进程级运行配置。
func TestBuildServiceContextDoesNotPublishRuntimeConfigOnFailure(t *testing.T) {
	prev := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: "stable-app"})
	t.Cleanup(func() {
		runtimecfg.Restore(prev)
	})

	svcCtx, shutdown, err := BuildServiceContext(context.Background(), config.Config{AppID: "failed-app"}, "failed-version")
	if err == nil {
		if svcCtx != nil {
			_ = bootstrapresources.CloseServiceContextResources(svcCtx)
		}
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		t.Fatal("期望缺少 MySQL 配置时启动失败")
	}
	if got := runtimecfg.AppID(); got != "stable-app" {
		t.Fatalf("启动失败后 runtimecfg.AppID() = %q, want stable-app", got)
	}
}

// TestCollectorConfigWithAppIDScopesRedisStream 确保 Collector Redis Stream 按 app_id 隔离。
func TestCollectorConfigWithAppIDScopesRedisStream(t *testing.T) {
	prev := runtimecfg.Get()
	runtimecfg.Set(config.Config{AppID: "site-1"})
	t.Cleanup(func() {
		runtimecfg.Restore(prev)
	})
	cfg := collectorConfigWithAppID(config.Config{
		AppID: "site-1",
		Collector: config.CollectorConfig{
			Redis: config.CollectorRedisConfig{Stream: "collector:events"},
		},
	})
	if got := cfg.Redis.Stream; got != "app:site-1:collector:events" {
		t.Fatalf("期望 Collector Redis Stream 按 app_id 加前缀，实际为 %q", got)
	}

	runtimecfg.Set(config.Config{AppID: "site-2"})
	cfg = collectorConfigWithAppID(config.Config{
		AppID: "site-2",
		Collector: config.CollectorConfig{
			Redis: config.CollectorRedisConfig{Stream: "app:site-1:collector:events"},
		},
	})
	if got := cfg.Redis.Stream; got != "" {
		t.Fatalf("期望已带其它 app 前缀的 Collector Redis Stream 失败闭合，实际为 %q", got)
	}
}
