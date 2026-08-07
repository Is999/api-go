package collectorx

import (
	"strings"
	"sync"

	"api/common/prometheusx"

	"github.com/Is999/go-utils/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector Prometheus 指标。
var (
	collectorMetricsOnce sync.Once // 保证 Collector 指标只注册一次
	collectorMetricsErr  error     // 保存启动期指标注册冲突，运行期记录遇到错误时直接跳过
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

// RegisterMetrics 注册 Collector 指标；同类型重复指标复用既有实例，冲突通过启动错误返回。
func RegisterMetrics() error {
	collectorMetricsOnce.Do(func() {
		collectorKafkaPublishEventsTotal, collectorMetricsErr = prometheusx.Register(collectorKafkaPublishEventsTotal)
	})
	return errors.Tag(collectorMetricsErr)
}

// recordKafkaPublish 记录一次 Kafka 投递结果。
func recordKafkaPublish(result string) {
	if RegisterMetrics() != nil {
		return
	}
	result = strings.TrimSpace(result)
	if result == "" {
		result = "unknown"
	}
	collectorKafkaPublishEventsTotal.WithLabelValues(result).Inc()
}
