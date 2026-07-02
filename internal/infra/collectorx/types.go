package collectorx

import (
	"context"
	"encoding/json"
	"time"
)

// Collector 内置业务类型常量。
const (
	// BizTypeAuthSecurity 表示认证风控事件的 Collector bizType。
	BizTypeAuthSecurity = "auth.security"
)

// Collector 运行异常类型常量。
const (
	// RuntimeAlertKindEnqueueFailed 表示 Collector 事件投递失败。
	RuntimeAlertKindEnqueueFailed = "collector_enqueue_failed"
	// RuntimeAlertKindProcessorFailed 表示 Collector Processor 处理失败。
	RuntimeAlertKindProcessorFailed = "collector_processor_failed"
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

// RuntimeAlert 描述 Collector 投递和同步处理链路中的运行异常。
type RuntimeAlert struct {
	Kind       string    // 异常类型，用于告警指纹和排障归类
	Title      string    // 告警标题
	Status     string    // 当前处理状态
	Component  string    // 发生异常的组件
	Operation  string    // 发生异常的操作
	BizType    string    // 关联业务类型
	Transport  string    // 事件投递通道
	UniqueKey  string    // 告警限频指纹
	Reason     string    // 异常原因摘要
	Advice     string    // 处理建议
	OccurredAt time.Time // 发现异常的时间
	Count      int       // 影响事件数量
}

// AlertHook 接收 Collector 运行异常；上层负责限频和外部通知。
type AlertHook func(ctx context.Context, alert RuntimeAlert)

// DefaultProcessorSpec 描述内置 Collector Processor 的注册规格和清单信息。
type DefaultProcessorSpec struct {
	Name        string           // 注册清单名称，必须唯一
	BizType     string           // Collector 业务类型，必须和事件 bizType 一致
	File        string           // 注册实现所在文件
	Method      string           // 注册入口方法或构造方法
	Description string           // 注册项中文说明
	Build       func() Processor // 构造默认 Processor，返回 nil 会被启动注册拦截
}

// ProcessorFunc 允许业务方用普通函数快速注册批量处理器。
type ProcessorFunc func(context.Context, []Event) ([]ProcessResult, error)

// ProcessBatch 执行批量消费函数。
func (f ProcessorFunc) ProcessBatch(ctx context.Context, events []Event) ([]ProcessResult, error) {
	return f(ctx, events)
}
