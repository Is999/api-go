package collectorx

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

const (
	collectorRuntimeChannelKafka    = "kafka"               // Kafka 是 API Collector 唯一正常投递通道
	defaultKafkaWriteTimeout        = 3 * time.Second       // 默认 Kafka 写入超时，避免认证主链路长期阻塞
	defaultKafkaWriteBatchSize      = 500                   // 默认 Producer 批量写入大小
	maxKafkaWriteBatchSize          = 5000                  // Producer 单批写入上限
	defaultKafkaWriteBatchWait      = 20 * time.Millisecond // 默认 Producer 批量等待时间
	maxKafkaWriteBatchWait          = 5 * time.Second       // Producer 批量等待时间上限
	maxKafkaWriteTimeout            = 30 * time.Second      // 请求链路 Producer 写入超时上限
	collectorDefaultRuntimeBizType  = "collector"           // 缺少 bizType 时的告警兜底业务类型
	collectorDefaultRuntimeUniqueID = "collector:kafka"     // 缺少稳定键时的告警兜底指纹
)

// Collector 事件上限与 Admin 消费及失败账本契约保持一致。
const (
	maxCollectorEventIDBytes      = 64        // 事件 ID 字节上限
	maxCollectorBizTypeBytes      = 100       // 业务类型字节上限
	maxCollectorPartitionKeyBytes = 128       // 分区键字节上限
	maxCollectorPayloadBytes      = 60 * 1024 // 事件 JSON 负载上限
	maxCollectorKafkaMessageBytes = 64 * 1024 // Kafka 完整事件信封上限
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
	normalizeCollectorConfig(&cfg)
	if err := RegisterMetrics(); err != nil {
		return nil, errors.Wrap(err, "注册 Collector 指标失败")
	}
	m := &Manager{
		cfg:     cfg,
		writers: make(map[string]kafkaMessageWriter),
	}
	if !cfg.Enabled || len(cfg.Kafka.Brokers) == 0 {
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

// Ready 检查 Collector 路由和 Kafka broker 是否可用。
func (m *Manager) Ready(ctx context.Context) error {
	if m == nil || !m.cfg.Enabled {
		return errors.New("collector 未初始化或未启用")
	}
	if len(m.writers) == 0 {
		return errors.New("collector Kafka writer 未初始化")
	}
	return errors.Tag(pingKafkaBrokers(ctx, m.cfg.Kafka.Brokers, collectorConfiguredTopics(m.cfg)))
}

// pingKafkaBrokers 通过 Kafka 元数据协议检查所有 Collector Topic。
func pingKafkaBrokers(ctx context.Context, brokers []string, topics []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	dialer := &kafka.Dialer{Timeout: time.Second}
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker == "" {
			continue
		}
		brokerReady := true
		for _, topic := range topics {
			partitions, err := dialer.LookupPartitions(ctx, "tcp", broker, topic)
			if err == nil && len(partitions) > 0 {
				continue
			}
			if err == nil {
				err = errors.Errorf("collector Kafka Topic不存在或无分区 topic=%s", topic)
			}
			lastErr = err
			brokerReady = false
			break
		}
		if brokerReady && len(topics) > 0 {
			return nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr == nil {
		return errors.New("collector Kafka broker 未配置")
	}
	return errors.Wrap(lastErr, "collector Kafka broker或Topic不可用")
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
			Channel:   collectorRuntimeChannelKafka,
			UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindEnqueueFailed, event.BizType, collectorRuntimeChannelKafka),
			Reason:    err.Error(),
			Advice:    "请检查 API collector.tasks 对应 bizType 的 topic 配置；修复后观察 Collector Kafka 投递指标。",
			Count:     1,
		})
		return "", err
	}
	body, err := json.Marshal(event)
	if err != nil {
		recordKafkaPublish("failed")
		return "", errors.Tag(err)
	}
	if len(body) > maxCollectorKafkaMessageBytes {
		recordKafkaPublish("failed")
		return "", errors.Errorf("collector Kafka 消息不能超过 %d 字节", maxCollectorKafkaMessageBytes)
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
			Channel:   collectorRuntimeChannelKafka,
			UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindEnqueueFailed, event.BizType, collectorRuntimeChannelKafka),
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
	closeFns := make([]func() error, 0, len(m.writers))
	for _, topic := range sortedWriterTopics(m.writers) {
		closeFns = append(closeFns, m.writers[topic].Close)
	}
	if err := closeAllWithContext(ctx, closeFns); err != nil {
		return errors.Wrap(err, "关闭 Collector Kafka writer 失败")
	}
	return nil
}

// closeAllWithContext 并发关闭彼此独立的 Kafka writer，并受应用停止期限约束。
func closeAllWithContext(ctx context.Context, closeFns []func() error) error {
	if len(closeFns) == 0 {
		return nil
	}
	errs := make(chan error, len(closeFns))
	var wg sync.WaitGroup
	wg.Add(len(closeFns))
	for _, closeFn := range closeFns {
		go func() {
			defer wg.Done()
			errs <- closeFn()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		close(errs)
		for err := range errs {
			if err != nil {
				return errors.Tag(err)
			}
		}
		return nil
	case <-ctx.Done():
		return errors.Tag(ctx.Err())
	}
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

// collectorTaskTopic 返回指定 bizType 显式配置的 Kafka Topic。
func (m *Manager) collectorTaskTopic(bizType string) string {
	if m == nil {
		return ""
	}
	bizType = strings.TrimSpace(bizType)
	task, ok := m.cfg.Tasks[bizType]
	if !ok {
		return ""
	}
	return strings.TrimSpace(task.Topic)
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
	alert.Title = strings.TrimSpace(alert.Title)
	alert.Status = strings.TrimSpace(alert.Status)
	alert.Component = strings.TrimSpace(alert.Component)
	if alert.Component == "" {
		alert.Component = "collector"
	}
	alert.Operation = strings.TrimSpace(alert.Operation)
	alert.BizType = strings.TrimSpace(alert.BizType)
	if alert.BizType == "" {
		alert.BizType = collectorDefaultRuntimeBizType
	}
	alert.Channel = strings.TrimSpace(alert.Channel)
	if alert.Channel == "" {
		alert.Channel = collectorRuntimeChannelKafka
	}
	alert.UniqueKey = strings.TrimSpace(alert.UniqueKey)
	if alert.UniqueKey == "" {
		alert.UniqueKey = collectorAlertUniqueKey(alert.Kind, alert.BizType, alert.Channel)
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
func collectorAlertUniqueKey(kind, bizType, channel string) string {
	parts := make([]string, 0, 3)
	for _, item := range []string{kind, bizType, channel} {
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

// normalizeAndValidateEvent 按 Admin 消费契约清洗并校验事件。
func normalizeAndValidateEvent(event *Event) error {
	if event == nil {
		return errors.Errorf("collector event 为空")
	}
	event.EventID = strings.TrimSpace(event.EventID)
	if event.EventID == "" {
		event.EventID = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	event.BizType = strings.TrimSpace(event.BizType)
	event.PartitionKey = strings.TrimSpace(event.PartitionKey)
	if len(event.Payload) == 0 {
		event.Payload = json.RawMessage("null")
	}
	if event.BizType == "" {
		return errors.Errorf("collector event bizType 为空")
	}
	if !utf8.ValidString(event.EventID) {
		return errors.Errorf("collector event eventId 不是有效 UTF-8")
	}
	if len(event.EventID) > maxCollectorEventIDBytes {
		return errors.Errorf("collector event eventId 不能超过 %d 字节", maxCollectorEventIDBytes)
	}
	if !utf8.ValidString(event.BizType) {
		return errors.Errorf("collector event bizType 不是有效 UTF-8")
	}
	if len(event.BizType) > maxCollectorBizTypeBytes {
		return errors.Errorf("collector event bizType 不能超过 %d 字节", maxCollectorBizTypeBytes)
	}
	if !utf8.ValidString(event.PartitionKey) {
		return errors.Errorf("collector event partitionKey 不是有效 UTF-8")
	}
	if len(event.PartitionKey) > maxCollectorPartitionKeyBytes {
		return errors.Errorf("collector event partitionKey 不能超过 %d 字节", maxCollectorPartitionKeyBytes)
	}
	if len(event.Payload) > maxCollectorPayloadBytes {
		return errors.Errorf("collector event payload 不能超过 %d 字节", maxCollectorPayloadBytes)
	}
	if !utf8.Valid(event.Payload) {
		return errors.Errorf("collector event payload 不是有效 UTF-8")
	}
	if !json.Valid(event.Payload) {
		return errors.Errorf("collector event payload 不是有效 JSON")
	}
	return nil
}

// normalizeCollectorConfig 清理 Kafka broker 和任务路由字段。
func normalizeCollectorConfig(cfg *config.CollectorConfig) {
	if cfg == nil {
		return
	}
	cfg.Kafka.Brokers = nonEmptyStrings(cfg.Kafka.Brokers)
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
	for _, task := range cfg.Tasks {
		topic := strings.TrimSpace(task.Topic)
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
	if timeout > maxKafkaWriteTimeout {
		return maxKafkaWriteTimeout
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
