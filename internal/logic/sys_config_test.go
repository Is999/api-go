package logic

import (
	"context"
	"reflect"
	"testing"

	"gozero_api/internal/config"
	"gozero_api/internal/model"
	"gozero_api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestDecodeSysConfigValue 校验系统配置缓存值按类型还原为业务值。
func TestDecodeSysConfigValue(t *testing.T) {
	tests := []struct {
		name string
		typ  int
		raw  string
		want any
	}{
		{name: "object", typ: model.SysConfigTypeObject, raw: `{"a":1}`, want: map[string]any{"a": float64(1)}},
		{name: "array", typ: model.SysConfigTypeArray, raw: `[1,"b"]`, want: []any{float64(1), "b"}},
		{name: "string_json", typ: model.SysConfigTypeString, raw: `"hello"`, want: "hello"},
		{name: "string_raw", typ: model.SysConfigTypeString, raw: `hello`, want: "hello"},
		{name: "integer", typ: model.SysConfigTypeInteger, raw: `42`, want: 42},
		{name: "float", typ: model.SysConfigTypeFloat, raw: `3.14`, want: 3.14},
		{name: "boolean", typ: model.SysConfigTypeBoolean, raw: `1`, want: true},
		{name: "group", typ: model.SysConfigTypeGroup, raw: `0`, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSysConfigValue(tt.typ, tt.raw)
			if err != nil {
				t.Fatalf("decodeSysConfigValue() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeSysConfigValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestSysConfigCacheKeyUsesAppNamespace 校验系统配置缓存按 app_id 精确隔离。
func TestSysConfigCacheKeyUsesAppNamespace(t *testing.T) {
	logicObj := NewSysConfigLogic(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{}))

	got := logicObj.sysConfigCacheKey("featureFlag")
	want := "app:site-a:config_uuid:featureFlag"
	if got != want {
		t.Fatalf("sysConfigCacheKey() = %q, want %q", got, want)
	}
}

// TestGetCachedValueReadsRedisBeforeDB 校验系统配置命中 Redis 后不会依赖数据库连接。
func TestGetCachedValueReadsRedisBeforeDB(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := NewSysConfigLogic(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{Rds: client}))
	cacheKey := logicObj.sysConfigCacheKey("featureFlag")
	if err := client.HSet(context.Background(), cacheKey, map[string]any{
		sysConfigCacheFieldUUID:  "featureFlag",
		sysConfigCacheFieldType:  model.SysConfigTypeBoolean,
		sysConfigCacheFieldValue: "1",
	}).Err(); err != nil {
		t.Fatalf("seed sys_config cache: %v", err)
	}

	value, err := logicObj.GetCachedValue("featureFlag")
	if err != nil {
		t.Fatalf("GetCachedValue() error = %v", err)
	}
	if value != true {
		t.Fatalf("GetCachedValue() = %#v, want true", value)
	}
}
