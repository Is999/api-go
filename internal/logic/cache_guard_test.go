package logic

import (
	"context"
	"testing"

	"api/internal/config"
	"api/internal/svc"
)

func TestCacheLockKeyUsesAppNamespace(t *testing.T) {
	useRuntimeAppID(t, "site-a")
	base := NewBaseLogicWithContext(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{}))
	got := base.cacheLockKey("app:site-a:config_uuid:featureFlag")
	want := "app:site-a:cache:rebuild:lock:config_uuid:featureFlag"
	if got != want {
		t.Fatalf("cacheLockKey() = %q, want %q", got, want)
	}
}

// TestRuntimeRegistrySpecsValid 确保通用缓存保护注册入口规格完整且名称唯一。
func TestRuntimeRegistrySpecsValid(t *testing.T) {
	specs := RuntimeRegistrySpecs()
	if len(specs) == 0 {
		t.Fatal("RuntimeRegistrySpecs() 不能为空")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.File == "" || spec.Method == "" || spec.Description == "" {
			t.Fatalf("运行时注册规格字段不完整: %+v", spec)
		}
		if _, ok := seen[spec.Name]; ok {
			t.Fatalf("运行时注册规格名称重复: %s", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}
