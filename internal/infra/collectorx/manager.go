package collectorx

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

const (
	collectorTransportKafka         = "kafka"               // Kafka 是 API Collector 唯一正常投递通道
	defaultKafkaWriteTimeout        = 3 * time.Second       // 默认 Kafka 写入超时，避免认证主链路长期阻塞
	defaultKafkaWriteBatchSize      = 500                   // 默认 Producer 批量写入大小
	maxKafkaWriteBatchSize          = 10000                 // Producer 单批写入上限
	defaultKafkaWriteBatchWait      = 20 * time.Millisecond // 默认 Producer 批量等待时间
	maxKafkaWriteBatchWait          = 5 * time.Second       // Producer 批量等待时间上限
	collectorDefaultRuntimeBizType  = "collector"           // 缺少 bizType 时的告警兜底业务类型
	collectorDefaultRuntimeUniqueID = "collector:kafka"     // 缺少稳定键时的告警兜底指纹
)

// kafkaMessageWriter 约束 Kafka 写入器能力，便于单测替换。
type kafkaMessageWriter interface {
	WriteMessages(context.Context, ...kafka.Message) error
	Close() error
}

// Manager 负责把 API 轻量事件可靠投递到 Kafka。
type Manager struct {
	cfg       config.CollectorConfig        // 收集器运行配置
	writers   map[string]kafkaMessageWriter // topic 到 Kafka writer 的映射
	alertHook AlertHook                     // 运行异常告警钩子
	mu        sync.RWMutex                  // 保护 alertHook
}

// New 创建通用收集器 Kafka 投递管理器。
func New(cfg config.CollectorConfig) (*Manager, error) {
	normalizeCollectorTaskRoutes(&cfg)
	ensureMetricsRegistered()
	m := &Manager{
		cfg:     cfg,
		writers: make(map[string]kafkaMessageWriter),
	}
	if !cfg.Enabled || len(nonEmptyStrings(cfg.Kafka.Brokers)) == 0 {
		return m, nil
	}
	for _, topic := range collectorConfiguredTopics(cfg) {
		m.writers[topic] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Kafka.Brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			BatchSize:    boundedPositiveInt(cfg.Kafka.WriteBatchSize, defaultKafkaWriteBatchSize, maxKafkaWriteBatchSize),
			BatchTimeout: kafkaWriteBatchWait(cfg.Kafka.WriteBatchWaitMilliseconds),
			WriteTimeout: kafkaWriteTimeout(cfg.Kafka.WriteTimeout),
			RequiredAcks: kafka.RequireAll,
			Async:        false,
		}
	}
	return m, nil
}

// SetAlertHook 设置 Collector 运行异常告警钩子。
func (m *Manager) SetAlertHook(hook AlertHook) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertHook = hook
}

// Enqueue 投递一条结构化业务事件到 Kafka。
func (m *Manager) Enqueue(ctx context.Context, event Event) (string, error) {
	if m == nil || !m.cfg.Enabled {
		return "", errors.Errorf("collector 未启用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := normalizeAndValidateEvent(&event); err != nil {
		recordKafkaPublish("failed")
		return "", errors.Tag(err)
	}
	topic := m.collectorTaskTopic(event.BizType)
	if strings.TrimSpace(topic) == "" {
		err := errors.Errorf("collector task topic 未配置 biz_type=%s", event.BizType)
		recordKafkaPublish("failed")
		m.reportRuntimeAlert(ctx, RuntimeAlert{
			Kind:      RuntimeAlertKindEnqueueFailed,
			Title:     "【P1 Collector 投递失败】",
			Status:    "Collector 事件未写入 Kafka，前台请求已继续处理",
			Component: "collector",
			Operation: "resolve_topic",
			BizType:   event.BizType,
			Transport: collectorTransportKafka,
			UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindEnqueueFailed, event.BizType, collectorTransportKafka),
			Reason:    err.Error(),
			Advice:    "请检查 API collector.tasks 或 collector.default_task 的 topic 配置；修复后观察 Collector Kafka 投递指标。",
			Count:     1,
		})
		return "", err
	}
	body, err := json.Marshal(event)
	if err != nil {
		recordKafkaPublish("failed")
		return "", errors.Tag(err)
	}
	if err = m.publishKafka(ctx, topic, kafkaMessageKey(event), body); err != nil {
		recordKafkaPublish("failed")
		m.reportRuntimeAlert(ctx, RuntimeAlert{
			Kind:      RuntimeAlertKindEnqueueFailed,
			Title:     "【P1 Collector 投递失败】",
			Status:    "Collector 事件未写入 Kafka，前台请求已继续处理",
			Component: "collector",
			Operation: "publish_kafka",
			BizType:   event.BizType,
			Transport: collectorTransportKafka,
			UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindEnqueueFailed, event.BizType, collectorTransportKafka),
			Reason:    err.Error(),
			Advice:    "请检查 Kafka broker、topic、ACK 和网络状态；修复后观察 Collector Kafka 投递指标。",
			Count:     1,
		})
		return "", errors.Tag(err)
	}
	recordKafkaPublish("success")
	return event.EventID, nil
}

// Close 关闭 Collector 持有的 Kafka writer。
func (m *Manager) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var firstErr error
	for _, topic := range sortedWriterTopics(m.writers) {
		select {
		case <-ctx.Done():
			if firstErr == nil {
				firstErr = errors.Tag(ctx.Err())
			}
			return firstErr
		default:
		}
		if err := m.writers[topic].Close(); err != nil && firstErr == nil {
			firstErr = errors.Wrapf(err, "关闭 Collector Kafka writer 失败 topic=%s", topic)
		}
	}
	return errors.Tag(firstErr)
}

// publishKafka 将事件写入 Kafka，并等待 broker ACK。
func (m *Manager) publishKafka(ctx context.Context, topic string, key string, body []byte) error {
	writer := m.writers[strings.TrimSpace(topic)]
	if writer == nil {
		return errors.Errorf("collector kafka topic 未配置 topic=%s", topic)
	}
	writeCtx, cancel := context.WithTimeout(ctx, kafkaWriteTimeout(m.cfg.Kafka.WriteTimeout))
	defer cancel()
	return errors.Tag(writer.WriteMessages(writeCtx, kafka.Message{Key: []byte(key), Value: body}))
}

// collectorTaskTopic 返回指定 bizType 继承后的 Kafka Topic。
func (m *Manager) collectorTaskTopic(bizType string) string {
	if m == nil {
		return ""
	}
	bizType = strings.TrimSpace(bizType)
	if task, ok := m.cfg.Tasks[bizType]; ok {
		return firstNonEmpty(task.Topic, m.cfg.DefaultTask.Topic)
	}
	return strings.TrimSpace(m.cfg.DefaultTask.Topic)
}

// reportRuntimeAlert 上报 Collector 运行异常；未设置 hook 时保持原返回语义。
func (m *Manager) reportRuntimeAlert(ctx context.Context, alert RuntimeAlert) {
	if m == nil {
		return
	}
	m.mu.RLock()
	hook := m.alertHook
	m.mu.RUnlock()
	if hook == nil {
		return
	}
	if alert.OccurredAt.IsZero() {
		alert.OccurredAt = time.Now()
	}
	hook(ctx, normalizeRuntimeAlert(alert))
}

// normalizeRuntimeAlert 清洗告警字段，避免空类型或高基数指纹。
func normalizeRuntimeAlert(alert RuntimeAlert) RuntimeAlert {
	alert.Kind = strings.TrimSpace(alert.Kind)
	if alert.Kind == "" {
		alert.Kind = RuntimeAlertKindEnqueueFailed
	}
	alert.Component = strings.TrimSpace(alert.Component)
	if alert.Component == "" {
		alert.Component = "collector"
	}
	alert.Operation = strings.TrimSpace(alert.Operation)
	alert.BizType = strings.TrimSpace(alert.BizType)
	if alert.BizType == "" {
		alert.BizType = collectorDefaultRuntimeBizType
	}
	alert.Transport = strings.TrimSpace(alert.Transport)
	if alert.Transport == "" {
		alert.Transport = collectorTransportKafka
	}
	alert.UniqueKey = strings.TrimSpace(alert.UniqueKey)
	if alert.UniqueKey == "" {
		alert.UniqueKey = collectorAlertUniqueKey(alert.Kind, alert.BizType, alert.Transport)
	}
	alert.Reason = strings.TrimSpace(alert.Reason)
	alert.Advice = strings.TrimSpace(alert.Advice)
	if alert.Count <= 0 {
		alert.Count = 1
	}
	if alert.OccurredAt.IsZero() {
		alert.OccurredAt = time.Now()
	}
	return alert
}

// collectorAlertUniqueKey 生成低基数告警指纹，避免事件 ID 导致 Lark 刷屏。
func collectorAlertUniqueKey(kind, bizType, transport string) string {
	parts := make([]string, 0, 3)
	for _, item := range []string{kind, bizType, transport} {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	if len(parts) == 0 {
		return collectorDefaultRuntimeUniqueID
	}
	return strings.Join(parts, ":")
}

// kafkaMessageKey 返回 Kafka 分区键，保证同任务同业务分区稳定进入同一 Kafka partition。
func kafkaMessageKey(event Event) string {
	bizType := strings.TrimSpace(event.BizType)
	partitionKey := strings.TrimSpace(event.PartitionKey)
	if partitionKey == "" {
		return bizType
	}
	return bizType + ":" + partitionKey
}

// normalizeAndValidateEvent 清洗事件并校验必要字段。
func normalizeAndValidateEvent(event *Event) error {
	if event == nil {
		return errors.Errorf("collector event 为空")
	}
	event.EventID = strings.TrimSpace(event.EventID)
	if event.EventID == "" {
		event.EventID = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	event.BizType = strings.TrimSpace(event.BizType)
	if event.BizType == "" {
		return errors.Errorf("collector event biz_type 为空")
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return errors.Errorf("collector event payload 必须是合法 JSON")
	}
	return nil
}

// normalizeCollectorTaskRoutes 清理任务 Kafka 路由字段，避免空白值参与匹配。
func normalizeCollectorTaskRoutes(cfg *config.CollectorConfig) {
	if cfg == nil {
		return
	}
	cfg.DefaultTask.Topic = strings.TrimSpace(cfg.DefaultTask.Topic)
	if len(cfg.Tasks) == 0 {
		return
	}
	tasks := make(map[string]config.CollectorTaskConfig, len(cfg.Tasks))
	for bizType, task := range cfg.Tasks {
		bizType = strings.TrimSpace(bizType)
		task.Topic = strings.TrimSpace(task.Topic)
		if bizType == "" {
			continue
		}
		tasks[bizType] = task
	}
	cfg.Tasks = tasks
}

// collectorConfiguredTopics 返回 Collector 当前配置需要写入的 Topic 列表。
func collectorConfiguredTopics(cfg config.CollectorConfig) []string {
	topics := make(map[string]struct{})
	if cfg.DefaultTask.Topic != "" {
		topics[cfg.DefaultTask.Topic] = struct{}{}
	}
	for _, task := range cfg.Tasks {
		topic := firstNonEmpty(task.Topic, cfg.DefaultTask.Topic)
		if topic == "" {
			continue
		}
		topics[topic] = struct{}{}
	}
	return sortedKeys(topics)
}

// kafkaWriteTimeout 返回 Producer 写入超时。
func kafkaWriteTimeout(seconds int) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 {
		return defaultKafkaWriteTimeout
	}
	return timeout
}

// kafkaWriteBatchWait 返回 Producer 写入批量等待时间。
func kafkaWriteBatchWait(milliseconds int) time.Duration {
	wait := time.Duration(milliseconds) * time.Millisecond
	if wait <= 0 {
		return defaultKafkaWriteBatchWait
	}
	if wait > maxKafkaWriteBatchWait {
		return maxKafkaWriteBatchWait
	}
	return wait
}

// boundedPositiveInt 将配置值限制在稳定范围内。
func boundedPositiveInt(value int, fallback int, maxValue int) int {
	if value <= 0 {
		return fallback
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
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

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// sortedKeys 返回 map key 的稳定排序结果。
func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// sortedWriterTopics 返回 writer topic 的稳定排序结果。
func sortedWriterTopics(writers map[string]kafkaMessageWriter) []string {
	topics := make([]string, 0, len(writers))
	for topic := range writers {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics
}
