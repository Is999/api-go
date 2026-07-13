package middleware

import (
	"testing"

	"api/internal/model"
)

// TestAuthVersionMatches 确保主库版本先提交、Redis 尚未清理时旧 JWT 仍会 fail-close。
func TestAuthVersionMatches(t *testing.T) {
	user := &model.User{ID: 42, AuthVersion: 2}
	if authVersionMatches(user, &UserTokenIdentity{UserID: 42, AuthVersion: 1}) {
		t.Fatal("authVersionMatches() = true for stale JWT")
	}
	if !authVersionMatches(user, &UserTokenIdentity{UserID: 42, AuthVersion: 2}) {
		t.Fatal("authVersionMatches() = false for current JWT")
	}
	if authVersionMatches(&model.User{ID: 42}, &UserTokenIdentity{UserID: 42}) {
		t.Fatal("authVersionMatches() = true for zero auth version")
	}
}
