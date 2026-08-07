package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"api/internal/config"
	"api/internal/requestctx"
	"api/internal/svc"
)

// TestRouteSpecRestRouteWritesRequestMeta 确保路由规格在进入 handler 前写入统一请求元数据。
func TestRouteSpecRestRouteWritesRequestMeta(t *testing.T) {
	ctx, _ := requestctx.New(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/live", nil).WithContext(ctx)
	called := false
	spec := RouteSpec{
		Method:        http.MethodGet,
		Path:          "/api/live",
		Meta:          HealthLive,
		Chain:         RouteSecurityNone,
		SkipAccessLog: true,
		Handler: func(_ *svc.ServiceContext) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				_ = w
				called = true
				meta := requestctx.FromContext(r.Context())
				if meta == nil {
					t.Fatal("请求元数据不能为空")
				}
				if meta.Route != string(HealthLive.Alias) {
					t.Fatalf("路由别名 = %q, want %q", meta.Route, HealthLive.Alias)
				}
				if !meta.SkipAccessLog {
					t.Fatal("期望路由规格写入跳过访问日志标记")
				}
			}
		},
	}

	service := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{})
	route, err := spec.RestRoute(service, nil, nil)
	if err != nil {
		t.Fatalf("RestRoute() error = %v", err)
	}
	route.Handler(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("期望执行路由 handler")
	}
}

// TestRouteSpecRestRouteRejectsSecurityBoundaryDrift 确保内网契约与安全链不一致时返回启动错误，不触发 panic。
func TestRouteSpecRestRouteRejectsSecurityBoundaryDrift(t *testing.T) {
	spec := RouteSpec{
		Method: http.MethodPost,
		Path:   "/internal/config-reload",
		Meta:   SystemConfigReloadRun,
		Chain:  RouteSecurityNone,
		Handler: func(*svc.ServiceContext) http.HandlerFunc {
			return func(http.ResponseWriter, *http.Request) {}
		},
	}

	service := svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{})
	if _, err := spec.RestRoute(service, nil, nil); err == nil {
		t.Fatal("路由访问类型与安全链不一致时必须返回启动错误")
	}
}
