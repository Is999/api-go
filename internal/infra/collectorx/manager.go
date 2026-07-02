package collectorx

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Collector 事件投递载体枚举常量。
const (
	collectorTransportAuto  = "auto"  // 自动选择载体：优先 Redis Stream，否则同步 Processor
	collectorTransportRedis = "redis" // Redis Stream 载体
	collectorTransportSync  = "sync"  // 同步 Processor 载体
)

// Manager 负责通用收集器的事件投递和 Processor 注册。
type Manager struct {
	cfg        config.CollectorConfig // 收集器运行配置
	redis      redis.UniversalClient  // Redis 客户端，用于 Redis Stream 载体
	alertHook  AlertHook              // 运行异常告警钩子
	mu         sync.RWMutex           // 保护 processors 注册表
	processors map[string]Processor   // bizType 到批量处理器的映射
}

// New 创建通用收集器管理器。
func New(cfg config.CollectorConfig, rds redis.UniversalClient) (*Manager, error) {
	cfg.Transport = normalizeCollectorTransport(cfg.Transport)
	cfg.Redis.Stream = strings.TrimSpace(cfg.Redis.Stream)
	cfg.Redis.Consumer = strings.TrimSpace(cfg.Redis.Consumer)
	ensureMetricsRegistered()
	return &Manager{
		cfg:        cfg,
		redis:      rds,
		processors: make(map[string]Processor),
	}, nil
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

// RegisterProcessor 注册指定 bizType 的批量消费处理器。
func (m *Manager) RegisterProcessor(bizType string, p Processor) error {
	if m == nil {
		return errors.Errorf("collector 未初始化")
	}
	bizType = strings.TrimSpace(bizType)
	if bizType == "" {
		return errors.Errorf("collectorx.RegisterProcessor bizType 为空")
	}
	if p == nil {
		return errors.Errorf("collectorx.RegisterProcessor processor 为空")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.processors[bizType]; ok {
		return errors.Errorf("collectorx.RegisterProcessor 重复注册 bizType=%s", bizType)
	}
	m.processors[bizType] = p
	allowMetricBizTypeLabel(bizType)
	return nil
}

// RegisterProcessorFunc 允许业务方直接传入批量消费函数。
func (m *Manager) RegisterProcessorFunc(bizType string, fn ProcessorFunc) error {
	if fn == nil {
		return errors.Errorf("collectorx.RegisterProcessorFunc processor 为空")
	}
	return errors.Tag(m.RegisterProcessor(bizType, fn))
}

// Enqueue 投递一条结构化业务事件。
func (m *Manager) Enqueue(ctx context.Context, event Event) (string, error) {
	if m == nil || !m.cfg.Enabled {
		return "", errors.Errorf("collector 未启用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := normalizeAndValidateEvent(&event); err != nil {
		recordCollectorEnqueue(normalizeCollectorTransport(m.cfg.Transport), "failed")
		return "", errors.Tag(err)
	}
	transport := normalizeCollectorTransport(m.cfg.Transport)
	if (transport == collectorTransportAuto || transport == collectorTransportRedis) && m.redisAvailable() {
		if err := m.publishRedis(ctx, event); err == nil {
			recordCollectorEnqueue(collectorTransportRedis, "success")
			return event.EventID, nil
		} else if transport == collectorTransportRedis {
			recordCollectorEnqueue(collectorTransportRedis, "failed")
			m.reportRuntimeAlert(ctx, RuntimeAlert{
				Kind:      RuntimeAlertKindEnqueueFailed,
				Title:     "【P1 Collector 投递失败】",
				Status:    "Collector 事件未写入 Redis Stream，前台请求已继续处理",
				Component: "collector",
				Operation: "publish_redis",
				BizType:   event.BizType,
				Transport: collectorTransportRedis,
				UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindEnqueueFailed, event.BizType, collectorTransportRedis),
				Reason:    err.Error(),
				Advice:    "请检查 Redis Stream 配置、Redis 连接和当前 app_id 命名空间；修复后确认认证风控事件指标恢复。",
				Count:     1,
			})
			return "", errors.Tag(err)
		}
	}
	if err := m.processSync(ctx, event); err != nil {
		recordCollectorEnqueue(collectorTransportSync, "failed")
		m.reportRuntimeAlert(ctx, RuntimeAlert{
			Kind:      RuntimeAlertKindProcessorFailed,
			Title:     "【P1 Collector 同步处理失败】",
			Status:    "Collector 同步 Processor 处理失败，当前事件未完成采集",
			Component: "collector",
			Operation: "process_sync",
			BizType:   event.BizType,
			Transport: collectorTransportSync,
			UniqueKey: collectorAlertUniqueKey(RuntimeAlertKindProcessorFailed, event.BizType, collectorTransportSync),
			Reason:    err.Error(),
			Advice:    "请检查对应 bizType 的 Processor 注册、业务依赖和最近发布；修复后观察 Collector 指标是否恢复。",
			Count:     1,
		})
		return "", errors.Tag(err)
	}
	recordCollectorEnqueue(collectorTransportSync, "success")
	return event.EventID, nil
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
		alert.Kind = RuntimeAlertKindProcessorFailed
	}
	alert.Component = strings.TrimSpace(alert.Component)
	if alert.Component == "" {
		alert.Component = "collector"
	}
	alert.Operation = strings.TrimSpace(alert.Operation)
	alert.BizType = strings.TrimSpace(alert.BizType)
	alert.Transport = strings.TrimSpace(alert.Transport)
	alert.UniqueKey = strings.TrimSpace(alert.UniqueKey)
	if alert.UniqueKey == "" {
		alert.UniqueKey = collectorAlertUniqueKey(alert.Kind, alert.BizType, alert.Transport)
	}
	alert.Reason = strings.TrimSpace(alert.Reason)
	alert.Advice = strings.TrimSpace(alert.Advice)
	if alert.Count <= 0 {
		alert.Count = 1
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
	return strings.Join(parts, ":")
}

// processSync 直接调用已注册 Processor，适合前台 API 的轻量收集场景。
func (m *Manager) processSync(ctx context.Context, event Event) error {
	m.mu.RLock()
	processor := m.processors[event.BizType]
	m.mu.RUnlock()
	if processor == nil {
		return errors.Errorf("collector processor 未注册 biz_type=%s", event.BizType)
	}
	begin := time.Now()
	results, err := processor.ProcessBatch(ctx, []Event{event})
	success := err == nil
	var resultErr error
	if err == nil && len(results) > 0 {
		success = results[0].Success
		if !success {
			resultErr = errors.Errorf("collector processor 处理失败 event_id=%s error=%s", results[0].EventID, results[0].Error)
		}
	}
	recordProcessorBatch(event.BizType, success, time.Since(begin))
	if resultErr != nil {
		return resultErr
	}
	return errors.Tag(err)
}

// redisAvailable 判断 Redis Stream 载体是否具备写入条件。
func (m *Manager) redisAvailable() bool {
	return m != nil && m.redis != nil && m.cfg.Redis.Enabled && strings.TrimSpace(m.cfg.Redis.Stream) != ""
}

// publishRedis 将事件写入 Redis Stream。
func (m *Manager) publishRedis(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return errors.Tag(err)
	}
	args := &redis.XAddArgs{
		Stream: m.cfg.Redis.Stream,
		Values: map[string]any{
			"body": string(body),
		},
	}
	if m.cfg.Redis.MaxLen > 0 {
		args.MaxLen = m.cfg.Redis.MaxLen
		args.Approx = true
	}
	return errors.Tag(m.redis.XAdd(ctx, args).Err())
}

// normalizeCollectorTransport 归一化配置中的 transport。
func normalizeCollectorTransport(transport string) string {
	value := strings.ToLower(strings.TrimSpace(transport))
	switch value {
	case "", collectorTransportAuto:
		return collectorTransportAuto
	case collectorTransportRedis:
		return collectorTransportRedis
	case collectorTransportSync:
		return collectorTransportSync
	default:
		return collectorTransportAuto
	}
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
