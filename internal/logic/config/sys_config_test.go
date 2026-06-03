package config

import (
	"context"
	"reflect"
	"testing"

	appconfig "api/internal/config"
	"api/internal/model"
	"api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestDecodeSysConfigValue 校验系统配置缓存值按类型还原为业务值。
func TestDecodeSysConfigValue(t *testing.T) {
	tests := []struct {
		name string // name 表示测试场景名称。
		typ  int    // typ 表示配置类型。
		raw  string // raw 表示原始输入值。
		want any    // want 表示期望结果。
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
	logicObj := NewSysConfigLogic(context.Background(), svc.NewServiceContext(appconfig.Config{AppID: "site-a"}, "v1", svc.Dependencies{}))

	got := logicObj.sysConfigCacheKey("featureFlag")
	want := "app:site-a:table:config_uuid:featureFlag"
	if got != want {
		t.Fatalf("sysConfigCacheKey() = %q, want %q", got, want)
	}
}

// TestGetCachedValueReadsRedisBeforeDB 校验系统配置命中 Redis 后不会依赖数据库连接。
func TestGetCachedValueReadsRedisBeforeDB(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := NewSysConfigLogic(context.Background(), svc.NewServiceContext(appconfig.Config{AppID: "site-a"}, "v1", svc.Dependencies{Rds: client}))
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

// TestGetCachedValueRejectsInvalidCacheType 确保损坏缓存不会被静默当成 group 类型。
func TestGetCachedValueRejectsInvalidCacheType(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := NewSysConfigLogic(context.Background(), svc.NewServiceContext(appconfig.Config{AppID: "site-a"}, "v1", svc.Dependencies{Rds: client}))
	cacheKey := logicObj.sysConfigCacheKey("featureFlag")
	if err := client.HSet(context.Background(), cacheKey, map[string]any{
		sysConfigCacheFieldUUID:  "featureFlag",
		sysConfigCacheFieldType:  "bad",
		sysConfigCacheFieldValue: "1",
	}).Err(); err != nil {
		t.Fatalf("seed sys_config cache: %v", err)
	}

	if _, err := logicObj.GetCachedValue("featureFlag"); err == nil {
		t.Fatal("GetCachedValue() expected invalid cache type error")
	}
}

// TestRuntimeRegistrySpecsValid 确保运行期配置注册入口规格完整且名称唯一。
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
