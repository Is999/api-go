package redisx

import (
	"context"
	"testing"

	"api/internal/config"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestNewEnablesContextTimeout 验证请求 deadline 会约束 Redis 命令等待时间。
func TestNewEnablesContextTimeout(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := New(context.Background(), config.RedisConfig{
		Type:     "single",
		Addrs:    []string{server.Addr()},
		PoolSize: 1,
	}, config.ObservabilityConfig{})
	if err != nil {
		t.Fatalf("创建 Redis 客户端失败: %v", err)
	}
	defer client.Close()

	singleClient, ok := client.(*redis.Client)
	if !ok || !singleClient.Options().ContextTimeoutEnabled {
		t.Fatalf("Redis 客户端必须启用 context deadline: type=%T", client)
	}
}
