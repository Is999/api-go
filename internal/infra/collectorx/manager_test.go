package collectorx

import (
	"context"
	"encoding/json"
	"testing"

	"api/internal/config"
)

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
