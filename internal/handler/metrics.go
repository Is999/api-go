package handler

import (
	"net/http"

	"gozero_api/internal/requestctx"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler 返回 Prometheus 指标抓取入口。
func MetricsHandler() http.HandlerFunc {
	handler := promhttp.Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(HealthMetrics.Alias))
		handler.ServeHTTP(w, r)
	}
}
