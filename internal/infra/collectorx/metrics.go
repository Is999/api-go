package collectorx

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector Prometheus 指标。
var (
	collectorMetricsOnce sync.Once // 保证 Collector 指标只注册一次
	// collectorKafkaPublishEventsTotal 统计 Collector Kafka 投递结果次数。
	collectorKafkaPublishEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "api",
			Subsystem: "collector",
			Name:      "kafka_publish_events_total",
			Help:      "Collector Kafka 投递事件累计数量。",
		},
		[]string{"result"},
	)
)

// ensureMetricsRegistered 保证 Prometheus 指标只注册一次。
func ensureMetricsRegistered() {
	collectorMetricsOnce.Do(func() {
		prometheus.MustRegister(
			collectorKafkaPublishEventsTotal,
		)
	})
}

// recordKafkaPublish 记录一次 Kafka 投递结果。
func recordKafkaPublish(result string) {
	ensureMetricsRegistered()
	result = strings.TrimSpace(result)
	if result == "" {
		result = "unknown"
	}
	collectorKafkaPublishEventsTotal.WithLabelValues(result).Inc()
}
