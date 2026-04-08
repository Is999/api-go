package logic

import (
	"context"
	"testing"

	codes "gozero_api/common/codes"
	"gozero_api/internal/config"
	"gozero_api/internal/svc"

	"github.com/Is999/go-utils/errors"
)

// TestReadinessUsesComponentRegistry 确保 ready 检查来自组件生命周期注册表。
func TestReadinessUsesComponentRegistry(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{})
	registry, err := svc.NewComponentRegistry(
		svc.Component{
			Name:      "mysql",
			ErrorCode: codes.MySQLUnavailable,
			Check: func(context.Context) error {
				return nil
			},
		},
		svc.Component{
			Name:      "redis",
			ErrorCode: codes.RedisUnavailable,
			Check: func(context.Context) error {
				return errors.Errorf("redis down")
			},
		},
	)
	if err != nil {
		t.Fatalf("NewComponentRegistry() error = %v", err)
	}
	svcCtx.SetComponentRegistry(registry)

	resp, err := NewHealthLogic(context.Background(), svcCtx).Readiness(context.Background())
	if err == nil {
		t.Fatal("expected readiness error")
	}
	if resp == nil || resp.Status != healthStatusError {
		t.Fatalf("readiness status = %+v, want error", resp)
	}
	if len(resp.Dependencies) != 2 {
		t.Fatalf("dependency count = %d, want 2", len(resp.Dependencies))
	}
	if resp.Dependencies[0].Name != "mysql" || resp.Dependencies[0].Status != healthStatusOK {
		t.Fatalf("mysql dependency = %+v", resp.Dependencies[0])
	}
	if resp.Dependencies[1].Name != "redis" || resp.Dependencies[1].Code != codes.RedisUnavailable {
		t.Fatalf("redis dependency = %+v", resp.Dependencies[1])
	}
}

// TestReadinessRejectsMissingComponentRegistry 确保组件清单缺失时不会误报 ready。
func TestReadinessRejectsMissingComponentRegistry(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{})
	resp, err := NewHealthLogic(context.Background(), svcCtx).Readiness(context.Background())
	if err == nil {
		t.Fatal("expected readiness error")
	}
	if resp == nil || resp.Status != healthStatusError || len(resp.Dependencies) != 1 {
		t.Fatalf("readiness response = %+v", resp)
	}
	if resp.Dependencies[0].Name != "component_registry" {
		t.Fatalf("dependency name = %s, want component_registry", resp.Dependencies[0].Name)
	}
}
