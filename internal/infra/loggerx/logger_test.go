package loggerx

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	goerrors "github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

// TestInfowSkipUsesOuterCallSite 验证调用方可以按需补充封装层数。
func TestInfowSkipUsesOuterCallSite(t *testing.T) {
	var buffer bytes.Buffer
	previousWriter := logx.Reset()
	logx.SetWriter(logx.NewWriter(&buffer))
	logx.SetLevel(logx.InfoLevel)
	t.Cleanup(func() {
		logx.SetWriter(previousWriter)
		logx.SetLevel(logx.InfoLevel)
	})

	expected := logInfoCallerSkipOuterProbe(context.Background())
	output := buffer.String()
	if !strings.Contains(output, expected) {
		t.Fatalf("caller should point to outer call site %s, got %s", expected, output)
	}
}

// logInfoCallerSkipOuterProbe 返回期望的外层业务调用点。
func logInfoCallerSkipOuterProbe(ctx context.Context) string {
	_, file, line, _ := runtime.Caller(0)
	expected := shortCaller(file, line+2)
	logInfoCallerSkipInnerProbe(ctx)
	return expected
}

// logInfoCallerSkipInnerProbe 模拟业务侧再包一层 loggerx 的调用。
func logInfoCallerSkipInnerProbe(ctx context.Context) {
	InfowSkip(ctx, 1, "loggerx caller skip probe")
}

// TestGoUtilsLoggerCallerUsesSourceCallSite 验证 go-utils 适配器 caller 落在日志产生处。
func TestGoUtilsLoggerCallerUsesSourceCallSite(t *testing.T) {
	var buffer bytes.Buffer
	previousWriter := logx.Reset()
	logx.SetWriter(logx.NewWriter(&buffer))
	logx.SetLevel(logx.InfoLevel)
	t.Cleanup(func() {
		logx.SetWriter(previousWriter)
		logx.SetLevel(logx.InfoLevel)
	})

	expected := logGoUtilsInfoCallerProbe()
	output := buffer.String()
	if !strings.Contains(output, expected) {
		t.Fatalf("go-utils caller should point to source call site %s, got %s", expected, output)
	}
	if strings.Contains(output, "loggerx/logger.go:") {
		t.Fatalf("go-utils caller should not point to loggerx wrapper, got %s", output)
	}
}

// logGoUtilsInfoCallerProbe 返回 go-utils 适配器应定位的日志产生点。
func logGoUtilsInfoCallerProbe() string {
	_, file, line, _ := runtime.Caller(0)
	expected := shortCaller(file, line+2)
	newGoUtilsLogger(nil).Info("go-utils caller probe")
	return expected
}

// TestErrorFieldsIncludeTraceAndCaller 验证错误字段包含可检索的链路文本和错误源位置。
func TestErrorFieldsIncludeTraceAndCaller(t *testing.T) {
	err := tracedErrorProbe()
	fields := fieldsToMap(ErrorFields(err))

	assertFieldValue(t, fields, fieldError, "boom")
	if !json.Valid([]byte(fields[fieldErrorChain])) {
		t.Fatalf("error_chain should be valid JSON, got %s", fields[fieldErrorChain])
	}
	if !strings.Contains(fields[fieldErrorChain], `"trace"`) {
		t.Fatalf("error_chain should include trace, got %s", fields[fieldErrorChain])
	}
	if !strings.Contains(fields[fieldErrorTrace], "loggerx/logger_test.go:") {
		t.Fatalf("error_trace should include source file, got %s", fields[fieldErrorTrace])
	}
	if !strings.Contains(fields[fieldErrorCaller], "loggerx/logger_test.go:") {
		t.Fatalf("error_caller should include source file, got %s", fields[fieldErrorCaller])
	}
}

// TestErrorCallerFieldsPreferErrorSource 验证错误日志 caller 指向错误源。
func TestErrorCallerFieldsPreferErrorSource(t *testing.T) {
	err := tracedErrorProbe()
	fields := fieldsToMap(appendErrorCallerFields(nil, err, "middleware/signature.go:233"))

	if !strings.Contains(fields[fieldCaller], "loggerx/logger_test.go:") {
		t.Fatalf("caller should prefer error source, got %s", fields[fieldCaller])
	}
	assertFieldValue(t, fields, fieldLogCaller, "middleware/signature.go:233")
}

// tracedErrorProbe 构造带 go-utils trace 的测试错误。
func tracedErrorProbe() error {
	return goerrors.Tag(stderrors.New("boom"))
}

// fieldsToMap 把日志字段转成便于断言的字符串 map。
func fieldsToMap(fields []logx.LogField) map[string]string {
	items := make(map[string]string, len(fields))
	for _, field := range fields {
		items[field.Key] = fmt.Sprint(field.Value)
	}
	return items
}

// assertFieldValue 断言指定日志字段存在且值匹配。
func assertFieldValue(t *testing.T, fields map[string]string, key, want string) {
	t.Helper()
	got, ok := fields[key]
	if !ok {
		t.Fatalf("expected key %q to exist", key)
	}
	if got != want {
		t.Fatalf("expected key %q to be %q, got %q", key, want, got)
	}
}
