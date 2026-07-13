package collectorx

import (
	"context"
	"encoding/json"
	"time"

	"api/internal/config"
)

// Collector 内置业务类型常量。
const (
	// BizTypeAuthSecurity 表示认证风控事件的 Collector bizType。
	BizTypeAuthSecurity = config.CollectorBizTypeAuthSecurity
)

// Collector 运行异常类型常量。
const (
	// RuntimeAlertKindEnqueueFailed 表示 Collector 事件投递失败。
	RuntimeAlertKindEnqueueFailed = "collector_enqueue_failed"
)

// Event 表示业务投递到通用收集器的一条结构化数据。
type Event struct {
	EventID      string          `json:"eventId"`      // 事件唯一 ID，空值由 Enqueue 生成，最多 64 字节
	BizType      string          `json:"bizType"`      // 业务类型，用于路由到对应 Kafka Topic，最多 100 字节
	PartitionKey string          `json:"partitionKey"` // 分区键或聚合键，最多 128 字节
	Payload      json.RawMessage `json:"payload"`      // 有效 UTF-8 JSON 负载，最多 60 KiB
}

// RuntimeAlert 描述 Collector Kafka 投递链路中的运行异常。
type RuntimeAlert struct {
	Kind       string    // 异常类型，用于告警指纹和排障归类
	Title      string    // 告警标题
	Status     string    // 当前处理状态
	Component  string    // 发生异常的组件
	Operation  string    // 发生异常的操作
	BizType    string    // 关联业务类型
	Channel    string    // 事件投递通道，当前为 Kafka
	UniqueKey  string    // 告警限频指纹
	Reason     string    // 异常原因摘要
	Advice     string    // 处理建议
	Count      int       // 影响事件数量
	OccurredAt time.Time // 发现异常的时间
}

// AlertHook 接收 Collector 运行异常；上层负责限频和外部通知。
type AlertHook func(ctx context.Context, alert RuntimeAlert)
