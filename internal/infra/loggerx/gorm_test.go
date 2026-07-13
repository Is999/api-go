package loggerx

import (
	"context"
	"testing"
)

// TestGormLoggerParamsFilterRemovesSensitiveValues 验证 GORM 只把占位 SQL 交给日志格式化器。
func TestGormLoggerParamsFilterRemovesSensitiveValues(t *testing.T) {
	logger := &GormLogger{}
	sql, params := logger.ParamsFilter(context.Background(), "SELECT * FROM user WHERE password_hash = ?", "secret-hash")
	if sql != "SELECT * FROM user WHERE password_hash = ?" || len(params) != 0 {
		t.Fatalf("ParamsFilter() sql=%q params=%v", sql, params)
	}
}
