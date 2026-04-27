package health

import (
	"net/http"

	"api/internal/handler/shared"
	"api/internal/requestctx"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler 返回 Prometheus 指标抓取入口。
func MetricsHandler() http.HandlerFunc {
	handler := promhttp.Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(shared.HealthMetrics.Alias))
		handler.ServeHTTP(w, r)
	}
}
