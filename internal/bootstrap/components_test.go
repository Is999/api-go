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
	got := componentItemNames(registry.Items())
	want := []string{componentNameMySQL, "mysql_archive", "mysql_user", componentNameRedis}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("component names = %v, want %v", got, want)
	}
}

// TestDefaultComponentSpecsValid 确保组件生命周期来源规格完整且顺序稳定。
func TestDefaultComponentSpecsValid(t *testing.T) {
	specs := defaultComponentSpecs()
	want := []string{componentNameMySQL, componentSourceSiteMySQL, componentNameRedis}
	if len(specs) != len(want) {
		t.Fatalf("默认组件规格数量不符合预期: got=%d want=%d", len(specs), len(want))
	}
	seen := make(map[string]struct{}, len(specs))
	for index, spec := range specs {
		if spec.Name != want[index] {
			t.Fatalf("默认组件规格顺序不符合预期: index=%d got=%s want=%s", index, spec.Name, want[index])
		}
		if spec.Build == nil {
			t.Fatalf("默认组件规格缺少构造函数: %s", spec.Name)
		}
		if spec.File == "" || spec.Method == "" || spec.Description == "" {
			t.Fatalf("默认组件规格清单字段不完整: %+v", spec)
		}
		if _, ok := seen[spec.Name]; ok {
			t.Fatalf("默认组件规格名称重复: %s", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}

// componentItemNames 保留组件注册顺序提取名称，便于校验生命周期顺序。
func componentItemNames(items []svc.Component) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
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
