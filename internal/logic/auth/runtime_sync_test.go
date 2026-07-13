package auth

import (
	"context"
	"testing"

	codes "api/common/codes"
	"api/internal/config"
	"api/internal/svc"
	"api/internal/types"
)

// TestSyncUserRuntimeDefaultsToProfileCache 验证未指定同步范围时默认只处理资料缓存。
func TestSyncUserRuntimeDefaultsToProfileCache(t *testing.T) {
	logicObj := NewAuthLogic(context.Background(), svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{}))
	resp := logicObj.SyncUserRuntime(&types.UserRuntimeSyncReq{ID: 42, Reason: "manual"})
	if resp.Code != codes.UpdateSuccess {
		t.Fatalf("SyncUserRuntime() code = %d, want %d", resp.Code, codes.UpdateSuccess)
	}
	data, ok := resp.Data.(*types.UserRuntimeSyncResp)
	if !ok {
		t.Fatalf("SyncUserRuntime() data type = %T, want *types.UserRuntimeSyncResp", resp.Data)
	}
	if data.UserID != 42 || !data.ProfileCacheInvalidated || data.SessionsInvalidated || data.Reason != "manual" {
		t.Fatalf("SyncUserRuntime() data = %+v, want profile-only sync", data)
	}
}

// TestSyncUserRuntimeSessionsRequireRedis 验证登录态失效必须由 API 进程持有 Redis 后才能执行。
func TestSyncUserRuntimeSessionsRequireRedis(t *testing.T) {
	logicObj := NewAuthLogic(context.Background(), svc.NewServiceContext(config.Config{}, "test-version", svc.Dependencies{}))
	resp := logicObj.SyncUserRuntime(&types.UserRuntimeSyncReq{ID: 42, Sessions: true, AuthVersion: 2})
	if resp.Code != codes.ServerError {
		t.Fatalf("SyncUserRuntime() code = %d, want %d when Redis missing", resp.Code, codes.ServerError)
	}
}
