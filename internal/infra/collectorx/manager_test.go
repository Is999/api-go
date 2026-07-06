package collectorx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/segmentio/kafka-go"
)

// fakeKafkaWriter 记录 Kafka 写入请求，避免单测依赖真实 broker。
type fakeKafkaWriter struct {
	messages []kafka.Message // 已写入的消息
	closeErr error           // Close 返回的错误
	writeErr error           // WriteMessages 返回的错误
	closed   bool            // 是否已关闭
}

// WriteMessages 记录本次写入。
func (f *fakeKafkaWriter) WriteMessages(_ context.Context, messages ...kafka.Message) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.messages = append(f.messages, messages...)
	return nil
}

// Close 标记 writer 已关闭。
func (f *fakeKafkaWriter) Close() error {
	f.closed = true
	return f.closeErr
}

// TestManagerEnqueuePublishesKafkaMessage 验证 Collector 按 bizType 和 partitionKey 写入 Kafka。
func TestManagerEnqueuePublishesKafkaMessage(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		DefaultTask: config.CollectorTaskConfig{Topic: "collector_events"},
	}, "collector_events")

	eventID, err := manager.Enqueue(context.Background(), Event{
		BizType:      " login ",
		PartitionKey: "user-1",
		Payload:      json.RawMessage(`{"uid":1}`),
	})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if eventID == "" {
		t.Fatal("event id is empty")
	}
	if len(writer.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(writer.messages))
	}
	if got := string(writer.messages[0].Key); got != "login:user-1" {
		t.Fatalf("message key = %q, want login:user-1", got)
	}
	var got Event
	if err := json.Unmarshal(writer.messages[0].Value, &got); err != nil {
		t.Fatalf("Unmarshal(message) error = %v", err)
	}
	if got.EventID != eventID || got.BizType != "login" {
		t.Fatalf("message event = %+v, want eventID=%s bizType=login", got, eventID)
	}
}

// TestManagerEnqueueUsesTaskTopic 验证不同 bizType 可投递到独立 Kafka Topic。
func TestManagerEnqueueUsesTaskTopic(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		DefaultTask: config.CollectorTaskConfig{Topic: "collector_events"},
		Tasks: map[string]config.CollectorTaskConfig{
			BizTypeAuthSecurity: {Topic: "auth_security_events"},
		},
	}, "auth_security_events")

	if _, err := manager.Enqueue(context.Background(), Event{
		BizType: BizTypeAuthSecurity,
		Payload: json.RawMessage(`{"action":"login_success"}`),
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if len(writer.messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(writer.messages))
	}
	if got := string(writer.messages[0].Key); got != BizTypeAuthSecurity {
		t.Fatalf("message key = %q, want %s", got, BizTypeAuthSecurity)
	}
}

// TestManagerEnqueueRejectsInvalidPayload 验证非法 JSON 负载不会写入 Kafka。
func TestManagerEnqueueRejectsInvalidPayload(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		DefaultTask: config.CollectorTaskConfig{Topic: "collector_events"},
	}, "collector_events")

	if _, err := manager.Enqueue(context.Background(), Event{
		BizType: "login",
		Payload: json.RawMessage(`{bad`),
	}); err == nil {
		t.Fatal("Enqueue() expected invalid payload error")
	}
	if len(writer.messages) != 0 {
		t.Fatalf("invalid payload should not publish, got %d messages", len(writer.messages))
	}
}

// TestManagerEnqueueReportsKafkaPublishAlert 验证 Kafka 写失败会上报低基数运行异常。
func TestManagerEnqueueReportsKafkaPublishAlert(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		DefaultTask: config.CollectorTaskConfig{Topic: "collector_events"},
	}, "collector_events")
	writer.writeErr = errors.Errorf("broker unavailable")

	var got RuntimeAlert
	manager.SetAlertHook(func(ctx context.Context, alert RuntimeAlert) {
		got = alert
	})
	_, err := manager.Enqueue(context.Background(), Event{
		BizType: BizTypeAuthSecurity,
		Payload: json.RawMessage(`{"uid":1}`),
	})
	if err == nil {
		t.Fatal("Enqueue() expected kafka error")
	}
	if got.Kind != RuntimeAlertKindEnqueueFailed {
		t.Fatalf("告警类型不符合预期: %+v", got)
	}
	if got.Component != "collector" || got.Operation != "publish_kafka" || got.Transport != collectorTransportKafka {
		t.Fatalf("告警归类不符合预期: %+v", got)
	}
	if got.BizType != BizTypeAuthSecurity || got.UniqueKey != "collector_enqueue_failed:auth.security:kafka" {
		t.Fatalf("告警指纹不符合预期: %+v", got)
	}
	if got.Count != 1 || got.OccurredAt.IsZero() {
		t.Fatalf("告警数量或时间不符合预期: %+v", got)
	}
	if !strings.Contains(got.Reason, "broker unavailable") {
		t.Fatalf("告警原因缺少错误摘要: %+v", got)
	}
}

// TestManagerCloseClosesWriters 验证关闭 Collector 会释放 Kafka writer。
func TestManagerCloseClosesWriters(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		DefaultTask: config.CollectorTaskConfig{Topic: "collector_events"},
	}, "collector_events")

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !writer.closed {
		t.Fatal("writer should be closed")
	}
}

// newKafkaManagerForTest 构造替换了 fake writer 的 Manager。
func newKafkaManagerForTest(t *testing.T, cfg config.CollectorConfig, topic string) (*Manager, *fakeKafkaWriter) {
	t.Helper()
	manager, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	writer := &fakeKafkaWriter{}
	manager.writers[topic] = writer
	return manager, writer
}
