package middleware

import (
	"strings"
	"testing"
)

// TestValidateSignValuesAllowsBoundedArray 验证显式签名策略允许小型数组按稳定 JSON 参与验签。
func TestValidateSignValuesAllowsBoundedArray(t *testing.T) {
	err := validateSignValues(map[string]any{
		"ids": []any{2.0, 3.0},
	}, []string{"ids"}, "请求签名")
	if err != nil {
		t.Fatalf("validateSignValues() error = %v, want nil", err)
	}
}

// TestValidateSignValuesRejectsOversizeArray 验证复杂签名字段仍受单字段大小上限保护。
func TestValidateSignValuesRejectsOversizeArray(t *testing.T) {
	err := validateSignValues(map[string]any{
		"ids": []any{strings.Repeat("1", 4096)},
	}, []string{"ids"}, "请求签名")
	if err == nil {
		t.Fatal("validateSignValues() error = nil, want payload limit error")
	}
}
