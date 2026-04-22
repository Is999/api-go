package middleware

import (
	"context"
	"testing"
	"time"

	"api/internal/config"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/redis/go-redis/v9"
)

// TestBearerToken 验证标准 Bearer token 提取。
func TestBearerToken(t *testing.T) {
	token, err := bearerToken("Bearer abc.def")
	if err != nil {
		t.Fatalf("bearerToken() error = %v", err)
	}
	if token != "abc.def" {
		t.Fatalf("bearerToken() = %q, want abc.def", token)
	}
}

// TestBearerTokenMissing 验证缺失 Bearer 前缀时返回错误。
func TestBearerTokenMissing(t *testing.T) {
	if _, err := bearerToken("Basic abc"); err == nil {
		t.Fatalf("bearerToken(Basic) error = nil, want error")
	}
}

// TestUserSessionKey 验证用户会话 Redis Key 模板。
func TestUserSessionKey(t *testing.T) {
	got := UserSessionKey("1", 42, "jti")
	want := "app:1:user:session:42:jti"
	if got != want {
		t.Fatalf("UserSessionKey() = %q, want %q", got, want)
	}
}

// TestUserSessionIndexKey 验证用户会话 jti 索引 Redis Key 模板。
func TestUserSessionIndexKey(t *testing.T) {
	got := UserSessionIndexKey("1", 42)
	want := "app:1:user:session:index:42"
	if got != want {
		t.Fatalf("UserSessionIndexKey() = %q, want %q", got, want)
	}
}

// TestVerifyUserTokenRejectsAppIDMismatch 确保 token 不能跨 AppID 复用。
func TestVerifyUserTokenRejectsAppIDMismatch(t *testing.T) {
	token := signedUserToken(t, "test-secret-please-change", "site-b")
	svcCtx := svc.NewServiceContext(config.Config{
		AppID:     "site-a",
		JwtSecret: "test-secret-please-change",
	}, "v1", svc.Dependencies{})

	if _, err := VerifyUserToken(context.Background(), svcCtx, token, false); !errors.Is(err, errInvalidToken) {
		t.Fatalf("VerifyUserToken() error = %v, want errInvalidToken", err)
	}
}

// TestVerifyUserTokenBackfillsSessionIndex 确保已有 session 鉴权成功后补齐 jti 索引。
func TestVerifyUserTokenBackfillsSessionIndex(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	token := signedUserToken(t, "test-secret-please-change", "site-a")
	sessionKey := UserSessionKey("site-a", 42, "testjti")
	if err := client.Set(context.Background(), sessionKey, token, time.Hour).Err(); err != nil {
		t.Fatalf("Set(session) error = %v", err)
	}
	svcCtx := svc.NewServiceContext(config.Config{
		AppID:     "site-a",
		JwtSecret: "test-secret-please-change",
	}, "v1", svc.Dependencies{Rds: client})

	identity, err := VerifyUserToken(context.Background(), svcCtx, token, true)
	if err != nil {
		t.Fatalf("VerifyUserToken() error = %v", err)
	}
	if identity.JTI != "testjti" {
		t.Fatalf("identity.JTI = %q, want testjti", identity.JTI)
	}
	members, err := client.ZRange(context.Background(), UserSessionIndexKey("site-a", 42), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange(index) error = %v", err)
	}
	if len(members) != 1 || members[0] != "testjti" {
		t.Fatalf("index members = %v, want [testjti]", members)
	}
	if ttl := client.TTL(context.Background(), UserSessionIndexKey("site-a", 42)).Val(); ttl <= 0 {
		t.Fatalf("index ttl = %v, want positive", ttl)
	}
}

// TestVerifyUserTokenReturnsIdentityOnSessionExpired 确保 session 失效时仍返回已校验 token 身份。
func TestVerifyUserTokenReturnsIdentityOnSessionExpired(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	token := signedUserToken(t, "test-secret-please-change", "site-a")
	svcCtx := svc.NewServiceContext(config.Config{
		AppID:     "site-a",
		JwtSecret: "test-secret-please-change",
	}, "v1", svc.Dependencies{Rds: client})

	identity, err := VerifyUserToken(context.Background(), svcCtx, token, true)
	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("VerifyUserToken() error = %v, want errSessionExpired", err)
	}
	if identity == nil || identity.UserID != 42 || identity.UserName != "demo" || identity.JTI != "testjti" {
		t.Fatalf("identity = %+v, want parsed token identity", identity)
	}
}

// TestVerifyUserTokenRejectsEmptyAppIDClaim 确保 token 必须携带明确 app_id。
func TestVerifyUserTokenRejectsEmptyAppIDClaim(t *testing.T) {
	token := signedUserToken(t, "test-secret-please-change", "")
	svcCtx := svc.NewServiceContext(config.Config{
		AppID:     "site-a",
		JwtSecret: "test-secret-please-change",
	}, "v1", svc.Dependencies{})

	if _, err := VerifyUserToken(context.Background(), svcCtx, token, false); !errors.Is(err, errInvalidToken) {
		t.Fatalf("VerifyUserToken() error = %v, want errInvalidToken", err)
	}
}

func signedUserToken(t *testing.T, secret string, appID string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":      42,
		"username": "demo",
		"jti":      "testjti",
		"app_id":   appID,
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}
