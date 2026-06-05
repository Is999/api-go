package bootstrap

import (
	"testing"

	"api/common/runtimecfg"
	"api/internal/config"
	"api/internal/svc"

	"gorm.io/gorm"
)

// TestBuildDefaultComponentRegistryIncludesCoreDependencies 确保核心依赖进入组件生命周期清单。
func TestBuildDefaultComponentRegistryIncludesCoreDependencies(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{
			NamedDBs: map[svc.DBName]*gorm.DB{
				svc.DBName("user"):    nil,
				svc.DBName("archive"): nil,
			},
		},
	})
	registry, err := buildDefaultComponentRegistry(svcCtx)
	if err != nil {
		t.Fatalf("buildDefaultComponentRegistry() error = %v", err)
	}
	got := componentItemNames(registry.Items())
	indexByName := make(map[string]int, len(got))
	for index, name := range got {
		indexByName[name] = index
	}
	for _, name := range []string{componentNameMySQL, "mysql_archive", "mysql_user", componentNameRedis} {
		if _, ok := indexByName[name]; !ok {
			t.Fatalf("组件生命周期清单缺少核心依赖 %q，实际为 %v", name, got)
		}
	}
	if indexByName["mysql_archive"] > indexByName["mysql_user"] {
		t.Fatalf("命名扩展库组件应按名称稳定排序，实际为 %v", got)
	}
}

// TestDefaultComponentSpecsValid 确保组件生命周期来源规格完整且名称唯一。
func TestDefaultComponentSpecsValid(t *testing.T) {
	specs := defaultComponentSpecs()
	if len(specs) == 0 {
		t.Fatal("默认组件规格不能为空")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
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
