package validators

import (
	"strings"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

const maxCollectorKafkaWriteBatchWaitMilliseconds = 5000 // API 请求链路内 Producer 聚合等待上限。

// ValidateCollector 校验 API Collector Kafka 投递配置是否自洽。
func ValidateCollector(c config.Config) error {
	cfg := c.Collector
	if !cfg.Enabled {
		return nil
	}
	if len(nonEmptyStrings(cfg.Kafka.Brokers)) == 0 {
		return errors.Errorf("collector.enabled=true 时必须配置 collector.kafka.brokers")
	}
	if cfg.Kafka.WriteBatchSize < 0 {
		return errors.Errorf("collector.kafka.write_batch_size 不能小于 0")
	}
	if cfg.Kafka.WriteBatchWaitMilliseconds < 0 || cfg.Kafka.WriteBatchWaitMilliseconds > maxCollectorKafkaWriteBatchWaitMilliseconds {
		return errors.Errorf("collector.kafka.write_batch_wait_milliseconds 必须在 0-%d 之间", maxCollectorKafkaWriteBatchWaitMilliseconds)
	}
	if cfg.Kafka.WriteTimeout < 0 {
		return errors.Errorf("collector.kafka.write_timeout 不能小于 0")
	}
	if strings.TrimSpace(cfg.DefaultTask.Topic) == "" && len(cfg.Tasks) == 0 {
		return errors.Errorf("collector.enabled=true 时必须配置 collector.default_task.topic 或 collector.tasks.<bizType>.topic")
	}
	for bizType, task := range cfg.Tasks {
		bizType = strings.TrimSpace(bizType)
		if bizType == "" {
			return errors.Errorf("collector.tasks 存在空 bizType")
		}
		if strings.TrimSpace(task.Topic) == "" && strings.TrimSpace(cfg.DefaultTask.Topic) == "" {
			return errors.Errorf("collector.tasks.%s.topic 不能为空", bizType)
		}
	}
	return nil
}

// nonEmptyStrings 返回去掉空白后的字符串列表。
func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
