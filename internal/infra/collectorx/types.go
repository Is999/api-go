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
)

// Event 表示业务投递到通用收集器的一条结构化数据。
type Event struct {
	EventID      string          `json:"eventId"`      // 事件唯一 ID，空值会由 Enqueue 自动生成
	BizType      string          `json:"bizType"`      // 业务类型，用于路由到对应 Kafka Topic
	PartitionKey string          `json:"partitionKey"` // 分区键或聚合键
	Payload      json.RawMessage `json:"payload"`      // 业务数据负载，必须是结构化 JSON
}

// RuntimeAlert 描述 Collector Kafka 投递链路中的运行异常。
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
