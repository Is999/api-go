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
			Enabled:     true,
			DefaultTask: config.CollectorTaskConfig{Topic: "api_collector_events"},
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
				"auth.security": {Topic: "api_collector_auth_security_events"},
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
			DefaultTask: config.CollectorTaskConfig{Topic: "api_collector_events"},
		},
	}
	if err := ValidateCollector(cfg); err == nil {
		t.Fatal("expected long collector.kafka.write_batch_wait_milliseconds to be rejected")
	}
}
