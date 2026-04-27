package logic

import (
	"context"
	"testing"

	"api/internal/config"
	"api/internal/svc"
)

func TestCacheLockKeyUsesAppNamespace(t *testing.T) {
	base := NewBaseLogicWithContext(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{}))
	got := base.cacheLockKey("app:site-a:config_uuid:featureFlag")
	want := "app:site-a:cache:rebuild:lock:config_uuid:featureFlag"
	if got != want {
		t.Fatalf("cacheLockKey() = %q, want %q", got, want)
	}
}
