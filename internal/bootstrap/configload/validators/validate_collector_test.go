package validators

import (
	"testing"

	"api/internal/config"
)

// TestValidateCollectorSkipsDisabledConfig 确保关闭 Collector 时不校验子配置细节。
func TestValidateCollectorSkipsDisabledConfig(t *testing.T) {
	cfg := config.Config{
		Collector: config.CollectorConfig{
			Enabled: false,
			Kafka: config.CollectorKafkaConfig{
				WriteBatchSize:             -1,
				WriteBatchWaitMilliseconds: 999999,
			},
		},
	}
	if err := ValidateCollector(cfg); err != nil {
		t.Fatalf("disabled collector should skip child validation: %v", err)
	}
}

// TestValidateCollectorRejectsMissingKafkaBrokers 确保启用 Collector 时必须配置 Kafka broker。
func TestValidateCollectorRejectsMissingKafkaBrokers(t *testing.T) {
	cfg := config.Config{
		Collector: config.CollectorConfig{
			Enabled: true,
			Tasks: map[string]config.CollectorTaskConfig{
				config.CollectorBizTypeAuthSecurity: {Topic: config.CollectorTopicAuthSecurity},
			},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected missing collector.kafka.brokers to be rejected")
	}
}

// TestValidateCollectorRejectsMissingTopic 确保启用 Collector 时必须有任务 Topic。
func TestValidateCollectorRejectsMissingTopic(t *testing.T) {
	cfg := config.Config{
		Collector: config.CollectorConfig{
			Enabled: true,
			Kafka: config.CollectorKafkaConfig{
				Brokers: []string{"127.0.0.1:9092"},
			},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected missing collector topic to be rejected")
	}
}

// TestValidateCollectorAcceptsTaskTopic 确保不同 bizType 可单独配置 Kafka Topic。
func TestValidateCollectorAcceptsTaskTopic(t *testing.T) {
	cfg := config.Config{
		Collector: config.CollectorConfig{
			Enabled: true,
			Kafka: config.CollectorKafkaConfig{
				Brokers: []string{"127.0.0.1:9092"},
			},
			Tasks: map[string]config.CollectorTaskConfig{
				config.CollectorBizTypeAuthSecurity: {Topic: config.CollectorTopicAuthSecurity},
			},
		},
	}
	if err := ValidateCollector(cfg); err != nil {
		t.Fatalf("valid task topic should pass: %v", err)
	}
}

// TestValidateCollectorRejectsInvalidKafkaWait 确保请求链路不会配置过长 Producer 等待。
func TestValidateCollectorRejectsInvalidKafkaWait(t *testing.T) {
	cfg := config.Config{
		Collector: config.CollectorConfig{
			Enabled: true,
			Kafka: config.CollectorKafkaConfig{
				Brokers:                    []string{"127.0.0.1:9092"},
				WriteBatchWaitMilliseconds: maxCollectorKafkaWriteBatchWaitMilliseconds + 1,
			},
			Tasks: map[string]config.CollectorTaskConfig{
				config.CollectorBizTypeAuthSecurity: {Topic: config.CollectorTopicAuthSecurity},
			},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected long collector.kafka.write_batch_wait_milliseconds to be rejected")
	}
}

// TestValidateCollectorRejectsInvalidKafkaBatchSize 确保 Producer 批量不会被静默截断。
func TestValidateCollectorRejectsInvalidKafkaBatchSize(t *testing.T) {
	cfg := validCollectorConfig()
	cfg.Collector.Kafka.WriteBatchSize = maxCollectorKafkaWriteBatchSize + 1
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected oversized collector.kafka.write_batch_size to be rejected")
	}
}

// TestValidateCollectorRejectsInvalidKafkaWriteTimeout 确保认证请求不会被超长 Kafka 超时占用。
func TestValidateCollectorRejectsInvalidKafkaWriteTimeout(t *testing.T) {
	cfg := validCollectorConfig()
	cfg.Collector.Kafka.WriteTimeout = maxCollectorKafkaWriteTimeoutSeconds + 1
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected long collector.kafka.write_timeout to be rejected")
	}
}

// TestValidateCollectorRejectsEmptyTaskTopic 确保每个业务类型都显式配置可用 Topic。
func TestValidateCollectorRejectsEmptyTaskTopic(t *testing.T) {
	cfg := validCollectorConfig()
	cfg.Collector.Tasks[config.CollectorBizTypeAuthSecurity] = config.CollectorTaskConfig{}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected empty collector task topic to be rejected")
	}
}

// validCollectorConfig 返回满足 API Collector 校验的最小配置。
func validCollectorConfig() config.Config {
	return config.Config{
		Collector: config.CollectorConfig{
			Enabled: true,
			Kafka: config.CollectorKafkaConfig{
				Brokers: []string{"127.0.0.1:9092"},
			},
			Tasks: map[string]config.CollectorTaskConfig{
				config.CollectorBizTypeAuthSecurity: {Topic: config.CollectorTopicAuthSecurity},
			},
		},
	}
}
