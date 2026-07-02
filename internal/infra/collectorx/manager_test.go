package collectorx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

// TestManagerEnqueueSyncProcessor 验证对应场景符合预期。
func TestManagerEnqueueSyncProcessor(t *testing.T) {
	manager, err := New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var seen Event
	if err := manager.RegisterProcessorFunc("login", func(ctx context.Context, events []Event) ([]ProcessResult, error) {
		if len(events) != 1 {
			t.Fatalf("events len = %d, want 1", len(events))
		}
		seen = events[0]
		return []ProcessResult{{EventID: events[0].EventID, Success: true}}, nil
	}); err != nil {
		t.Fatalf("RegisterProcessorFunc() error = %v", err)
	}

	eventID, err := manager.Enqueue(context.Background(), Event{
		BizType: " login ",
		Payload: json.RawMessage(`{"uid":1}`),
	})
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if eventID == "" || seen.EventID != eventID {
		t.Fatalf("event id not propagated, got=%q seen=%q", eventID, seen.EventID)
	}
	if seen.BizType != "login" {
		t.Fatalf("biz type = %q, want login", seen.BizType)
	}
}

// TestManagerEnqueueRejectsInvalidPayload 验证对应场景符合预期。
func TestManagerEnqueueRejectsInvalidPayload(t *testing.T) {
	manager, err := New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := manager.Enqueue(context.Background(), Event{
		BizType: "login",
		Payload: json.RawMessage(`{bad`),
	}); err == nil {
		t.Fatal("Enqueue() expected invalid payload error")
	}
}

// TestManagerEnqueueReportsProcessorAlert 验证同步 Processor 失败会上报低基数运行异常。
func TestManagerEnqueueReportsProcessorAlert(t *testing.T) {
	manager, err := New(config.CollectorConfig{
		Enabled:   true,
		Transport: "sync",
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := manager.RegisterProcessorFunc("auth.security", func(ctx context.Context, events []Event) ([]ProcessResult, error) {
		return nil, errors.Errorf("db timeout event_id=%s", events[0].EventID)
	}); err != nil {
		t.Fatalf("RegisterProcessorFunc() error = %v", err)
	}

	var got RuntimeAlert
	manager.SetAlertHook(func(ctx context.Context, alert RuntimeAlert) {
		got = alert
	})
	_, err = manager.Enqueue(context.Background(), Event{
		BizType: "auth.security",
		Payload: json.RawMessage(`{"uid":1}`),
	})
	if err == nil {
		t.Fatal("Enqueue() expected processor error")
	}
	if got.Kind != RuntimeAlertKindProcessorFailed {
		t.Fatalf("告警类型不符合预期: %+v", got)
	}
	if got.Component != "collector" || got.Operation != "process_sync" || got.Transport != collectorTransportSync {
		t.Fatalf("告警归类不符合预期: %+v", got)
	}
	if got.BizType != "auth.security" || got.UniqueKey != "collector_processor_failed:auth.security:sync" {
		t.Fatalf("告警指纹不符合预期: %+v", got)
	}
	if got.Count != 1 || got.OccurredAt.IsZero() {
		t.Fatalf("告警数量或时间不符合预期: %+v", got)
	}
	if !strings.Contains(got.Reason, "db timeout") {
		t.Fatalf("告警原因缺少错误摘要: %+v", got)
	}
}

// TestRuntimeRegistrySpecsValid 确保 Collector 运行时注册入口规格完整且名称唯一。
func TestRuntimeRegistrySpecsValid(t *testing.T) {
	specs := RuntimeRegistrySpecs()
	if len(specs) == 0 {
		t.Fatal("RuntimeRegistrySpecs() 不能为空")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.File == "" || spec.Method == "" || spec.Description == "" {
			t.Fatalf("运行时注册规格字段不完整: %+v", spec)
		}
		if _, ok := seen[spec.Name]; ok {
			t.Fatalf("运行时注册规格名称重复: %s", spec.Name)
		}
		seen[spec.Name] = struct{}{}
	}
}
