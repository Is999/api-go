package larkx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api/internal/config"
)

// requireCardText 返回卡片内所有可见文本，便于断言关键排障字段。
func requireCardText(t *testing.T, payload messagePayload) string {
	t.Helper()
	if payload.Card == nil {
		t.Fatalf("payload card is nil: %+v", payload)
	}
	return cardTextContent(*payload.Card)
}

// cardTextContent 汇总卡片标题、正文和字段文本。
func cardTextContent(card messageCard) string {
	parts := make([]string, 0, 8)
	if card.Header != nil {
		parts = append(parts, card.Header.Title.Content)
	}
	for _, element := range card.Elements {
		if element.Text != nil {
			parts = append(parts, element.Text.Content)
		}
		for _, field := range element.Fields {
			parts = append(parts, field.Text.Content)
		}
		for _, item := range element.Elements {
			parts = append(parts, item.Content)
		}
	}
	return strings.Join(parts, "\n")
}

// TestSendRuntimeAlertBuildsProfessionalCard 验证 API 运行异常告警使用可排障的 Lark 卡片。
func TestSendRuntimeAlertBuildsProfessionalCard(t *testing.T) {
	var got messagePayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	notifier, err := New(config.LarkAlertConfig{
		Enabled:        true,
		WebhookURL:     server.URL,
		Secret:         "secret",
		TimeoutSeconds: 1,
		MaxErrorBytes:  80,
		AtAll:          true,
	})
	if err != nil {
		t.Fatalf("new notifier failed: %v", err)
	}
	notifier.client = server.Client()
	notifier.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	err = notifier.SendRuntimeAlert(context.Background(), RuntimeAlert{
		ServiceName:  "api",
		Environment:  "pro",
		AppID:        "1",
		Kind:         "config_reload_failed",
		Title:        "【P1 API 配置热加载失败】",
		Status:       "配置热加载未生效",
		Component:    "config_reload",
		Operation:    "load",
		BizType:      "manual_api",
		Transport:    "config.yaml",
		UniqueKey:    "manual_api:load",
		OccurredAt:   time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC),
		Reason:       "配置文件格式错误\nline=8",
		Advice:       "修复配置后重新触发热加载",
		TriggerCount: 3,
	})
	if err != nil {
		t.Fatalf("send runtime alert failed: %v", err)
	}
	if got.MsgType != "interactive" {
		t.Fatalf("msg_type=%s, want interactive", got.MsgType)
	}
	if got.Timestamp != "1700000000" || got.Sign != sign("1700000000", "secret") {
		t.Fatalf("unexpected signed payload: timestamp=%s sign=%s", got.Timestamp, got.Sign)
	}
	if got.Card == nil || got.Card.Header == nil || got.Card.Header.Template != "red" {
		t.Fatalf("unexpected runtime alert card: %+v", got.Card)
	}
	text := requireCardText(t, got)
	for _, want := range []string{
		"P1 API 配置热加载失败",
		"**状态**：配置热加载未生效",
		"**服务**\napi",
		"**环境 / 站点**\npro / 1",
		"**类型**\nconfig_reload_failed",
		"**组件 / 动作**\nconfig_reload / load",
		"**业务类型**\nmanual_api",
		"**通道**\nconfig.yaml",
		"**去重键**\nmanual_api:load",
		"**窗口触发次数**\n3",
		"**错误摘要**\n配置文件格式错误 line=8",
		"**处理建议**\n- 修复配置后重新触发热加载",
		`<at id=all></at>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("card text missing %q:\n%s", want, text)
		}
	}
}
