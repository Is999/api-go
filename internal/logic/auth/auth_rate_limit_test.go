package auth

import (
	"context"
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
	err := logicObj.checkAuthRateLimit(authRateLimitActionLoginIP, "127.0.0.1", cfg)
	if !errors.Is(err, ErrAuthRateLimited) {
		t.Fatalf("second checkAuthRateLimit() error = %v, want ErrAuthRateLimited", err)
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
	_ = logicObj.checkAuthRateLimit(authRateLimitActionLoginUsername, subject, cfg)
	_ = logicObj.checkAuthRateLimit(authRateLimitActionLoginUsername, subject, cfg)
	logicObj.clearAuthRateLimit(authRateLimitActionLoginUsername, subject)

	if err := logicObj.checkAuthRateLimit(authRateLimitActionLoginUsername, subject, cfg); err != nil {
		t.Fatalf("checkAuthRateLimit() after clear error = %v", err)
	}
}

func newAuthLogicForRateLimit(client redis.UniversalClient) *AuthLogic {
	return NewAuthLogic(context.Background(), svc.NewServiceContext(config.Config{AppID: "site-a"}, "v1", svc.Dependencies{Rds: client}))
}
