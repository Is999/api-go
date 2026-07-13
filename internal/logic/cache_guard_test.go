package logic

import (
	"testing"

	keys "api/common/rediskeys"
)

// TestCacheIsEmptyMarkerUsesCanonicalValue 验证只识别当前统一的空值标记。
func TestCacheIsEmptyMarkerUsesCanonicalValue(t *testing.T) {
	tests := []struct {
		name  string // name 表示测试场景。
		value string // value 表示待识别的缓存值。
		want  bool   // want 表示是否应识别为空值标记。
	}{
		{name: "current marker", value: keys.EmptyValueMarker, want: true},
		{name: "non-canonical marker", value: "__EMPTY__", want: false},
		{name: "regular value", value: `{"id":1}`, want: false},
		{name: "empty string", value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CacheIsEmptyMarker(tt.value); got != tt.want {
				t.Fatalf("CacheIsEmptyMarker(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
