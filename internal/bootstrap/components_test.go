package bootstrap

import (
	"reflect"
	"testing"

	"gozero_api/internal/config"
	"gozero_api/internal/svc"

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
