package larkx

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// RuntimeAlert 描述一次 API 运行异常告警。
type RuntimeAlert struct {
	ServiceName  string    // 服务名
	Environment  string    // 运行环境
	AppID        string    // 站点/应用 ID
	Kind         string    // 异常类型
	Title        string    // 告警标题
	Status       string    // 当前处理状态
	Component    string    // 发生异常的组件
	Operation    string    // 发生异常的操作
	BizType      string    // 关联业务类型
	Transport    string    // 关联传输通道
	UniqueKey    string    // 告警限频指纹
	OccurredAt   time.Time // 发现时间
	Reason       string    // 异常原因
	Advice       string    // 处理建议
	TriggerCount int       // 当前告警窗口累计触发次数，包含本次触发
}

// SendRuntimeAlert 发送 API 运行异常告警。
func (n *Notifier) SendRuntimeAlert(ctx context.Context, alert RuntimeAlert) error {
	if n == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return n.sendCard(ctx, n.formatRuntimeAlertCard(alert))
}

// formatRuntimeAlertCard 构造 API 运行异常告警卡片。
func (n *Notifier) formatRuntimeAlertCard(alert RuntimeAlert) messageCard {
	occurredAt := alert.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = n.now()
	}
	status := strings.TrimSpace(alert.Status)
	if status == "" {
		status = "API 外层运行操作失败，需要人工确认"
	}
	elements := []messageCardElement{
		cardMarkdown("**状态**：%s\n**发现时间**：%s", status, formatCardTime(occurredAt)),
		cardFieldsCompact([][2]string{
			{"服务", alert.ServiceName},
			{"环境 / 站点", joinNonEmpty(" / ", alert.Environment, alert.AppID)},
			{"类型", alert.Kind},
			{"组件 / 动作", joinNonEmpty(" / ", alert.Component, alert.Operation)},
			{"业务类型", alert.BizType},
			{"通道", alert.Transport},
			{"去重键", alert.UniqueKey},
			{"窗口触发次数", triggerCountText(alert.TriggerCount)},
		}),
	}
	if reason := n.truncateText(alert.Reason); reason != "" {
		elements = append(elements, cardMarkdown("**错误摘要**\n%s", shortCardText(reason, n.maxErrorByte)))
	}
	elements = append(elements, cardMarkdown("**处理建议**\n%s", runtimeAlertAdviceText(n, alert.Advice)))
	if n.atAll {
		elements = append(elements, cardMarkdown("<at id=all></at>"))
	}
	return messageCard{
		Config: messageCardConfig{WideScreenMode: true},
		Header: &messageCardHeader{
			Template: "red",
			Title: messageCardText{
				Tag:     "plain_text",
				Content: runtimeAlertCardTitle(alert.Title),
			},
		},
		Elements: elements,
	}
}

// runtimeAlertCardTitle 规范化运行异常卡片标题。
func runtimeAlertCardTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "P1 API 运行异常"
	}
	title = strings.TrimPrefix(title, "【")
	title = strings.TrimSuffix(title, "】")
	return strings.TrimSpace(title)
}

// runtimeAlertAdviceText 生成运行异常处理建议。
func runtimeAlertAdviceText(n *Notifier, advice string) string {
	if advice := n.truncateText(advice); advice != "" {
		return "- " + advice
	}
	return "- 检查 API 配置、数据库、Redis、Collector 和最近发布变更\n- 修复后观察健康检查、错误日志和相关指标是否恢复"
}

// triggerCountText 只在重复触发时展示窗口触发次数。
func triggerCountText(count int) string {
	if count <= 1 {
		return ""
	}
	return strconv.Itoa(count)
}
