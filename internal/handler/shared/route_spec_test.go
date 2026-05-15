package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"api/internal/middleware"
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
		SkipAccessLog: true,
		Handler: func(_ *svc.ServiceContext, _ *middleware.AuthMiddleware) http.HandlerFunc {
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

	route := spec.RestRoute(nil, nil)
	route.Handler(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("期望执行路由 handler")
	}
}
