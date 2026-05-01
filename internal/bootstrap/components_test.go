package bootstrap

import (
	"reflect"
	"testing"

	"api/common/runtimecfg"
	"api/internal/config"
	"api/internal/svc"

	"gorm.io/gorm"
)

// TestBuildDefaultComponentRegistryNames 确保核心依赖进入组件生命周期清单。
func TestBuildDefaultComponentRegistryNames(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{
			NamedDBs: map[svc.DbName]*gorm.DB{
				svc.DbName("user"):    nil,
				svc.DbName("archive"): nil,
			},
		},
	})
	registry, err := buildDefaultComponentRegistry(svcCtx)
	if err != nil {
		t.Fatalf("buildDefaultComponentRegistry() error = %v", err)
	}
	got := componentNames(registry)
	want := []string{"mysql", "mysql_archive", "mysql_user", "redis"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("component names = %v, want %v", got, want)
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
