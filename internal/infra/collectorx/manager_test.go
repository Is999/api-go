package collectorx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/segmentio/kafka-go"
)

// fakeKafkaWriter 记录 Kafka 写入请求，避免单测依赖真实 broker。
type fakeKafkaWriter struct {
	messages   []kafka.Message // 已写入的消息
	closeErr   error           // Close 返回的错误
	closeBlock <-chan struct{} // 非空时阻塞 Close，用于停止期限测试
	writeErr   error           // WriteMessages 返回的错误
	closed     bool            // 是否已关闭
}

// TestManagerReadyRejectsMissingKafkaWriter 确保启用 Collector 后没有可用 Topic 时不会误报就绪。
func TestManagerReadyRejectsMissingKafkaWriter(t *testing.T) {
	manager, err := New(config.CollectorConfig{Enabled: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err = manager.Ready(context.Background()); err == nil {
		t.Fatal("Ready() expected missing Kafka writer error")
	}
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
	if f.closeBlock != nil {
		<-f.closeBlock
	}
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
		Tasks: map[string]config.CollectorTaskConfig{
			"login": {Topic: "collector_events"},
		},
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
		Tasks: map[string]config.CollectorTaskConfig{
			"login": {Topic: "collector_events"},
		},
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

// TestManagerEnqueueRejectsEventsOutsideContract 确保 API 不会写出 Admin 必然拒绝的事件。
func TestManagerEnqueueRejectsEventsOutsideContract(t *testing.T) {
	oversizedPayload := json.RawMessage(`"` + strings.Repeat("a", maxCollectorPayloadBytes-1) + `"`)
	escapedMessagePayload := json.RawMessage(`"` + strings.Repeat("<", 12000) + `"`)
	tests := []struct {
		name  string // 测试场景名称
		event Event  // 待投递的非法事件
		want  string // 期望错误包含的边界信息
	}{
		{name: "event id 字节超限", event: Event{EventID: strings.Repeat("界", 22), BizType: "login"}, want: "64 字节"},
		{name: "event id UTF-8 无效", event: Event{EventID: string([]byte{0xff}), BizType: "login"}, want: "UTF-8"},
		{name: "biz type 字节超限", event: Event{EventID: "event-1", BizType: strings.Repeat("b", maxCollectorBizTypeBytes+1)}, want: "100 字节"},
		{name: "partition key 字节超限", event: Event{EventID: "event-1", BizType: "login", PartitionKey: strings.Repeat("p", maxCollectorPartitionKeyBytes+1)}, want: "128 字节"},
		{name: "payload 字节超限", event: Event{EventID: "event-1", BizType: "login", Payload: oversizedPayload}, want: "61440 字节"},
		{name: "payload JSON 无效", event: Event{EventID: "event-1", BizType: "login", Payload: json.RawMessage(`{`)}, want: "JSON"},
		{name: "payload UTF-8 无效", event: Event{EventID: "event-1", BizType: "login", Payload: json.RawMessage{'"', 0xff, '"'}}, want: "UTF-8"},
		{name: "Kafka 信封超限", event: Event{EventID: "event-1", BizType: "login", Payload: escapedMessagePayload}, want: "65536 字节"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
				Enabled: true,
				Kafka: config.CollectorKafkaConfig{
					Brokers: []string{"127.0.0.1:9092"},
				},
				Tasks: map[string]config.CollectorTaskConfig{
					"login": {Topic: "collector_events"},
				},
			}, "collector_events")
			if _, err := manager.Enqueue(context.Background(), tt.event); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Enqueue() err=%v, want contains %q", err, tt.want)
			}
			if len(writer.messages) != 0 {
				t.Fatalf("越界事件不应写入 Kafka，messages=%d", len(writer.messages))
			}
		})
	}
}

// TestManagerEnqueueAcceptsContractBoundary 确保 API 与 Admin 对精确事件边界的判断一致。
func TestManagerEnqueueAcceptsContractBoundary(t *testing.T) {
	event := Event{
		EventID:      strings.Repeat("界", 21) + "e",
		BizType:      strings.Repeat("类", 33) + "b",
		PartitionKey: strings.Repeat("键", 42) + "pp",
		Payload:      json.RawMessage(`"` + strings.Repeat("a", maxCollectorPayloadBytes-2) + `"`),
	}
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		Tasks: map[string]config.CollectorTaskConfig{
			event.BizType: {Topic: "collector_events"},
		},
	}, "collector_events")

	if _, err := manager.Enqueue(context.Background(), event); err != nil {
		t.Fatalf("Enqueue() boundary event error = %v", err)
	}
	if len(writer.messages) != 1 || len(writer.messages[0].Value) > maxCollectorKafkaMessageBytes {
		t.Fatalf("boundary message count=%d bytes=%d", len(writer.messages), len(writer.messages[0].Value))
	}
}

// TestManagerEnqueueRejectsUnconfiguredBizType 确保 API 不使用默认 Topic 掩盖缺失的业务路由。
func TestManagerEnqueueRejectsUnconfiguredBizType(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		Tasks: map[string]config.CollectorTaskConfig{
			"login": {Topic: "collector_events"},
		},
	}, "collector_events")

	if _, err := manager.Enqueue(context.Background(), Event{BizType: "unknown"}); err == nil || !strings.Contains(err.Error(), "topic 未配置") {
		t.Fatalf("Enqueue() err=%v, want missing topic", err)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("未配置业务类型不应写入 Kafka，messages=%d", len(writer.messages))
	}
}

// TestNewNormalizesKafkaBrokers 确保 writer 和 readiness 使用清理后的 broker 地址。
func TestNewNormalizesKafkaBrokers(t *testing.T) {
	manager, err := New(config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{" kafka:9092 ", " "},
		},
		Tasks: map[string]config.CollectorTaskConfig{
			"login": {Topic: "collector_events"},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(manager.cfg.Kafka.Brokers) != 1 || manager.cfg.Kafka.Brokers[0] != "kafka:9092" {
		t.Fatalf("Kafka brokers = %#v", manager.cfg.Kafka.Brokers)
	}
}

// TestManagerEnqueueReportsKafkaPublishAlert 验证 Kafka 写失败会上报低基数运行异常。
func TestManagerEnqueueReportsKafkaPublishAlert(t *testing.T) {
	manager, writer := newKafkaManagerForTest(t, config.CollectorConfig{
		Enabled: true,
		Kafka: config.CollectorKafkaConfig{
			Brokers: []string{"127.0.0.1:9092"},
		},
		Tasks: map[string]config.CollectorTaskConfig{
			BizTypeAuthSecurity: {Topic: "collector_events"},
		},
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
	if got.Component != "collector" || got.Operation != "publish_kafka" || got.Channel != collectorRuntimeChannelKafka {
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
		Tasks: map[string]config.CollectorTaskConfig{
			"login": {Topic: "collector_events"},
		},
	}, "collector_events")

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !writer.closed {
		t.Fatal("writer should be closed")
	}
}

// TestManagerCloseHonorsContextDeadline 确保 Kafka 关闭阻塞时不会突破应用停止期限。
func TestManagerCloseHonorsContextDeadline(t *testing.T) {
	closeBlock := make(chan struct{})
	manager := &Manager{writers: map[string]kafkaMessageWriter{
		"collector_events": &fakeKafkaWriter{closeBlock: closeBlock},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.Close(ctx); err == nil {
		close(closeBlock)
		t.Fatal("Close() expected context deadline error")
	}
	close(closeBlock)
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
