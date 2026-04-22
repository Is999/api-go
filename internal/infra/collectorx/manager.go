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

const (
	collectorTransportAuto  = "auto"  // 自动选择载体：优先 Redis Stream，否则同步 Processor
	collectorTransportRedis = "redis" // Redis Stream 载体
	collectorTransportSync  = "sync"  // 同步 Processor 载体
)

// Manager 负责通用收集器的事件投递和 Processor 注册。
type Manager struct {
	cfg        config.CollectorConfig // 收集器运行配置
	redis      redis.UniversalClient  // Redis 客户端，用于 Redis Stream 载体
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
			return "", errors.Tag(err)
		}
	}
	if err := m.processSync(ctx, event); err != nil {
		recordCollectorEnqueue(collectorTransportSync, "failed")
		return "", errors.Tag(err)
	}
	recordCollectorEnqueue(collectorTransportSync, "success")
	return event.EventID, nil
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
