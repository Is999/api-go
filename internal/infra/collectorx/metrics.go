package collectorx

import (
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector 指标标签保护常量。
const (
	maxCollectorMetricBizTypeLabels = 64      // 限制动态 biz_type label 数量，避免指标维度失控
	collectorMetricBizTypeOther     = "other" // 超出上限或未知 bizType 统一归并到 other
)

// Collector Prometheus 指标和动态标签保护状态。
var (
	collectorMetricsOnce        sync.Once
	collectorMetricBizTypeGuard = struct {
		mu      sync.RWMutex        // 保护 biz_type label 集合
		allowed map[string]struct{} // 已明确放行的业务类型标签
	}{
		allowed: make(map[string]struct{}),
	}

	collectorEnqueueEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "api",
			Subsystem: "collector",
			Name:      "enqueue_events_total",
			Help:      "Collector 入队事件累计数量。",
		},
		[]string{"transport", "result"},
	)
	collectorProcessorBatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "api",
			Subsystem: "collector",
			Name:      "processor_batch_duration_seconds",
			Help:      "Collector 单次 Processor 批量处理耗时分布。",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5},
		},
		[]string{"biz_type", "result"},
	)
	authSecurityEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "api",
			Subsystem: "auth_security",
			Name:      "events_total",
			Help:      "认证风控事件累计数量。",
		},
		[]string{"app_id", "action", "reason", "category"},
	)
)

// ensureMetricsRegistered 保证 Prometheus 指标只注册一次。
func ensureMetricsRegistered() {
	collectorMetricsOnce.Do(func() {
		prometheus.MustRegister(
			collectorEnqueueEventsTotal,
			collectorProcessorBatchDuration,
			authSecurityEventsTotal,
		)
	})
}

// allowMetricBizTypeLabel 显式登记允许暴露的 bizType 指标维度。
func allowMetricBizTypeLabel(bizType string) {
	value := strings.TrimSpace(bizType)
	if value == "" {
		return
	}
	collectorMetricBizTypeGuard.mu.Lock()
	defer collectorMetricBizTypeGuard.mu.Unlock()
	collectorMetricBizTypeGuard.allowed[value] = struct{}{}
}

// normalizeBizTypeMetricLabel 对 biz_type 指标维度做白名单和上限保护。
func normalizeBizTypeMetricLabel(bizType string) string {
	value := strings.TrimSpace(bizType)
	if value == "" {
		return "unknown"
	}
	collectorMetricBizTypeGuard.mu.RLock()
	_, ok := collectorMetricBizTypeGuard.allowed[value]
	currentSize := len(collectorMetricBizTypeGuard.allowed)
	collectorMetricBizTypeGuard.mu.RUnlock()
	if ok {
		return value
	}
	collectorMetricBizTypeGuard.mu.Lock()
	defer collectorMetricBizTypeGuard.mu.Unlock()
	if _, ok = collectorMetricBizTypeGuard.allowed[value]; ok {
		return value
	}
	if len(collectorMetricBizTypeGuard.allowed) >= maxCollectorMetricBizTypeLabels && currentSize >= maxCollectorMetricBizTypeLabels {
		return collectorMetricBizTypeOther
	}
	collectorMetricBizTypeGuard.allowed[value] = struct{}{}
	return value
}

// recordCollectorEnqueue 记录一次事件入队结果。
func recordCollectorEnqueue(transport string, result string) {
	ensureMetricsRegistered()
	transport = strings.TrimSpace(transport)
	if transport == "" {
		transport = "unknown"
	}
	result = strings.TrimSpace(result)
	if result == "" {
		result = "unknown"
	}
	collectorEnqueueEventsTotal.WithLabelValues(transport, result).Inc()
}

// recordProcessorBatch 记录一次 Processor 批处理耗时。
func recordProcessorBatch(bizType string, success bool, duration time.Duration) {
	ensureMetricsRegistered()
	result := "success"
	if !success {
		result = "failed"
	}
	collectorProcessorBatchDuration.WithLabelValues(normalizeBizTypeMetricLabel(bizType), result).Observe(duration.Seconds())
}

// recordAuthSecurityEvent 记录认证风控事件聚合指标。
func recordAuthSecurityEvent(appID string, action string, reason string) {
	ensureMetricsRegistered()
	normalizedReason := normalizeAuthSecurityReason(reason)
	authSecurityEventsTotal.WithLabelValues(
		normalizeAuthSecurityAppID(appID),
		normalizeAuthSecurityAction(action),
		normalizedReason,
		normalizeAuthSecurityCategory(reason),
	).Inc()
}
