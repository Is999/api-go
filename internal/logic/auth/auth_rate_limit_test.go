package auth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"api/internal/config"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestCheckAuthRateLimitLocksAfterMaxAttempts 确保认证入口超过阈值后进入锁定状态。
func TestCheckAuthRateLimitLocksAfterMaxAttempts(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForRateLimit(client)
	cfg := config.AuthRateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxAttempts:   1,
		LockSeconds:   60,
	}
	if err := logicObj.checkAuthRateLimit(authRateLimitActionLoginIP, "127.0.0.1", cfg); err != nil {
		t.Fatalf("first checkAuthRateLimit() error = %v", err)
	}
	countKey, lockKey := logicObj.authRateLimitKeys(authRateLimitActionLoginIP, "127.0.0.1")
	if ttl := client.TTL(context.Background(), countKey).Val(); ttl <= 0 {
		t.Fatalf("rate limit count ttl = %v, want positive", ttl)
	}
	if countKey != "app:site-a:auth:rate_limit:{login_ip:f528764d624db129b32c21fbca0cb8d6}:count" ||
		lockKey != "app:site-a:auth:rate_limit:{login_ip:f528764d624db129b32c21fbca0cb8d6}:lock" {
		t.Fatalf("rate limit keys = %q %q, want same hash tag", countKey, lockKey)
	}
	err := logicObj.checkAuthRateLimit(authRateLimitActionLoginIP, "127.0.0.1", cfg)
	if !errors.Is(err, ErrAuthRateLimited) {
		t.Fatalf("second checkAuthRateLimit() error = %v, want ErrAuthRateLimited", err)
	}
	if exists := client.Exists(context.Background(), countKey).Val(); exists != 0 {
		t.Fatalf("rate limit count exists = %d, want deleted after lock", exists)
	}
}

// TestCheckAuthRateLimitConcurrentBoundary 确保并发请求不会越过原子最大尝试次数。
func TestCheckAuthRateLimitConcurrentBoundary(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	logicObj := newAuthLogicForRateLimit(client)
	cfg := config.AuthRateLimitConfig{Enabled: true, WindowSeconds: 60, MaxAttempts: 10, LockSeconds: 60}

	var allowed atomic.Int64
	var unexpected atomic.Int64
	var workers sync.WaitGroup
	for index := 0; index < 50; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := logicObj.checkAuthRateLimit(authRateLimitActionLoginIP, "127.0.0.2", cfg)
			switch {
			case err == nil:
				allowed.Add(1)
			case !errors.Is(err, ErrAuthRateLimited):
				unexpected.Add(1)
			}
		}()
	}
	workers.Wait()
	if unexpected.Load() != 0 {
		t.Fatalf("unexpected errors = %d, want 0", unexpected.Load())
	}
	if allowed.Load() != int64(cfg.MaxAttempts) {
		t.Fatalf("allowed requests = %d, want %d", allowed.Load(), cfg.MaxAttempts)
	}
}

// TestClearAuthRateLimitRemovesCountAndLock 确保登录成功后可以清理当前主体的限流状态。
func TestClearAuthRateLimitRemovesCountAndLock(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForRateLimit(client)
	cfg := config.AuthRateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxAttempts:   1,
		LockSeconds:   60,
	}
	subject := "demo_user"
	_ = logicObj.checkAuthRateLimit(authRateLimitActionLoginIdentity, subject, cfg)
	_ = logicObj.checkAuthRateLimit(authRateLimitActionLoginIdentity, subject, cfg)
	logicObj.clearAuthRateLimit(authRateLimitActionLoginIdentity, subject)

	if err := logicObj.checkAuthRateLimit(authRateLimitActionLoginIdentity, subject, cfg); err != nil {
		t.Fatalf("checkAuthRateLimit() after clear error = %v", err)
	}
}

// newAuthLogicForRateLimit 构造测试依赖。
func newAuthLogicForRateLimit(client redis.UniversalClient) *AuthLogic {
	return NewAuthLogic(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{Rds: client}))
}
