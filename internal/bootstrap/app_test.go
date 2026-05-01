package bootstrap

import (
	"context"
	"testing"

	"api/common/runtimecfg"
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
			_ = closeServiceContextResources(svcCtx)
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
