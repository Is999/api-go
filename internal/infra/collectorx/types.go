package collectorx

import (
	"context"
	"encoding/json"
)

// Collector 内置业务类型常量。
const (
	// BizTypeAuthSecurity 表示认证风控事件的 Collector bizType。
	BizTypeAuthSecurity = "auth.security"
)

// Event 表示业务投递到通用收集器的一条结构化数据。
type Event struct {
	EventID      string          `json:"eventId"`      // 事件唯一 ID，空值会由 Enqueue 自动生成
	BizType      string          `json:"bizType"`      // 业务类型，用于路由 Processor
	PartitionKey string          `json:"partitionKey"` // 分区键或聚合键
	Payload      json.RawMessage `json:"payload"`      // 业务数据负载，必须是结构化 JSON
}

// ProcessResult 表示批量处理器对单个事件的处理结果。
type ProcessResult struct {
	EventID string // 事件唯一 ID，必须对应输入事件
	Success bool   // 是否处理成功
	Error   string // 失败原因摘要
}

// Processor 定义业务批量消费接口。
type Processor interface {
	ProcessBatch(context.Context, []Event) ([]ProcessResult, error)
}

// ProcessorFunc 允许业务方用普通函数快速注册批量处理器。
type ProcessorFunc func(context.Context, []Event) ([]ProcessResult, error)

// ProcessBatch 执行批量消费函数。
func (f ProcessorFunc) ProcessBatch(ctx context.Context, events []Event) ([]ProcessResult, error) {
	return f(ctx, events)
}
