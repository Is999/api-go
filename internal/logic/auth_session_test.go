package logic

import (
	"context"
	stderrors "errors"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gozero_api/internal/config"
	"gozero_api/internal/model"
	"gozero_api/internal/svc"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestCreateSessionWritesSessionIndex 确保创建会话时同步记录用户 jti 索引。
func TestCreateSessionWritesSessionIndex(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForSession(client, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}
	created, err := logicObj.createSessionWithJTI(user)
	if err != nil {
		t.Fatalf("createSessionWithJTI() error = %v", err)
	}

	members, err := client.ZRange(context.Background(), logicObj.userSessionIndexKey(user.ID), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange(index) error = %v", err)
	}
	if len(members) != 1 || members[0] != created.JTI {
		t.Fatalf("index members = %v, want [%s]", members, created.JTI)
	}
	if ttl := client.TTL(context.Background(), logicObj.userSessionIndexKey(user.ID)).Val(); ttl <= 0 {
		t.Fatalf("index ttl = %v, want positive", ttl)
	}
}

// TestRotateSessionDeletesOldSession 确保刷新 token 后旧 jti 会话立即失效。
func TestRotateSessionDeletesOldSession(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForSession(client, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}
	oldToken, _, err := logicObj.generateJWT(user, "oldjti")
	if err != nil {
		t.Fatalf("generateJWT() error = %v", err)
	}
	oldKey := logicObj.userSessionKey(user.ID, "oldjti")
	if err := client.Set(context.Background(), oldKey, oldToken, time.Hour).Err(); err != nil {
		t.Fatalf("Set(old session) error = %v", err)
	}

	resp, err := logicObj.rotateSession(user, "oldjti")
	if err != nil {
		t.Fatalf("rotateSession() error = %v", err)
	}
	if err := client.Get(context.Background(), oldKey).Err(); !stderrors.Is(err, redis.Nil) {
		t.Fatalf("old session err = %v, want redis.Nil", err)
	}
	newJTI := tokenJTI(resp.Token, logicObj.svc.CurrentConfig().JwtSecret)
	if newJTI == "" || newJTI == "oldjti" {
		t.Fatalf("new jti = %q, want non-empty and different", newJTI)
	}
	if err := client.Get(context.Background(), logicObj.userSessionKey(user.ID, newJTI)).Err(); err != nil {
		t.Fatalf("new session missing: %v", err)
	}
	members, err := client.ZRange(context.Background(), logicObj.userSessionIndexKey(user.ID), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange(index) error = %v", err)
	}
	if len(members) != 1 || members[0] != newJTI {
		t.Fatalf("index members = %v, want only new jti %q", members, newJTI)
	}
}

// TestRotateSessionRollsBackNewSessionOnOldDeleteFailure 确保旧 session 删除失败时不泄露新 session。
func TestRotateSessionRollsBackNewSessionOnOldDeleteFailure(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	rds := &failFirstDelRedis{UniversalClient: client}
	logicObj := newAuthLogicForSession(rds, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}
	oldKey := logicObj.userSessionKey(user.ID, "oldjti")
	if err := client.Set(context.Background(), oldKey, "old-token", time.Hour).Err(); err != nil {
		t.Fatalf("Set(old session) error = %v", err)
	}

	if _, err := logicObj.rotateSession(user, "oldjti"); err == nil {
		t.Fatal("rotateSession() error = nil, want delete failure")
	}
	sessionKeys := sessionKeysForUser(server.Keys(), user.ID)
	if len(sessionKeys) != 1 || sessionKeys[0] != oldKey {
		t.Fatalf("session keys = %v, want only old session %q", sessionKeys, oldKey)
	}
}

// TestDeleteUserSessionRemovesIndex 确保删除单个 session 时同步移除 jti 索引。
func TestDeleteUserSessionRemovesIndex(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForSession(client, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}
	created, err := logicObj.createSessionWithJTI(user)
	if err != nil {
		t.Fatalf("createSessionWithJTI() error = %v", err)
	}

	if err := logicObj.deleteUserSession(user.ID, created.JTI); err != nil {
		t.Fatalf("deleteUserSession() error = %v", err)
	}
	if err := client.Get(context.Background(), logicObj.userSessionKey(user.ID, created.JTI)).Err(); !stderrors.Is(err, redis.Nil) {
		t.Fatalf("session err = %v, want redis.Nil", err)
	}
	if members := client.ZRange(context.Background(), logicObj.userSessionIndexKey(user.ID), 0, -1).Val(); len(members) != 0 {
		t.Fatalf("index members = %v, want empty", members)
	}
}

// TestInvalidateUserSessionsDeletesIndexedSessions 确保可按 jti 索引精确删除用户全部 session。
func TestInvalidateUserSessionsDeletesIndexedSessions(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	logicObj := newAuthLogicForSession(client, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}
	first, err := logicObj.createSessionWithJTI(user)
	if err != nil {
		t.Fatalf("createSessionWithJTI(first) error = %v", err)
	}
	second, err := logicObj.createSessionWithJTI(user)
	if err != nil {
		t.Fatalf("createSessionWithJTI(second) error = %v", err)
	}

	members := client.ZRange(context.Background(), logicObj.userSessionIndexKey(user.ID), 0, -1).Val()
	sort.Strings(members)
	wantMembers := []string{first.JTI, second.JTI}
	sort.Strings(wantMembers)
	for index := range wantMembers {
		if len(members) != len(wantMembers) || members[index] != wantMembers[index] {
			t.Fatalf("index members = %v, want %v", members, wantMembers)
		}
	}

	if err := logicObj.InvalidateUserSessions(user.ID); err != nil {
		t.Fatalf("InvalidateUserSessions() error = %v", err)
	}
	for _, jti := range wantMembers {
		if err := client.Get(context.Background(), logicObj.userSessionKey(user.ID, jti)).Err(); !stderrors.Is(err, redis.Nil) {
			t.Fatalf("session[%s] err = %v, want redis.Nil", jti, err)
		}
	}
	if exists := client.Exists(context.Background(), logicObj.userSessionIndexKey(user.ID)).Val(); exists != 0 {
		t.Fatalf("session index exists = %d, want 0", exists)
	}
}

// TestCreateSessionRollsBackWhenSessionIndexFails 确保索引写入失败时回滚已写 session。
func TestCreateSessionRollsBackWhenSessionIndexFails(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	rds := &failZAddRedis{UniversalClient: client}
	logicObj := newAuthLogicForSession(rds, config.AuthConfig{SessionTTLSeconds: 60})
	user := &model.APIUser{ID: 42, Username: "demo", Status: model.APIUserStatusEnabled}

	if _, err := logicObj.createSessionWithJTI(user); err == nil {
		t.Fatal("createSessionWithJTI() error = nil, want index failure")
	}
	if keys := sessionKeysForUser(server.Keys(), user.ID); len(keys) != 0 {
		t.Fatalf("session keys = %v, want empty after rollback", keys)
	}
}

// TestSessionTTLDoesNotExceedJWT 确保 Redis 会话 TTL 不超过 JWT 过期时间。
func TestSessionTTLDoesNotExceedJWT(t *testing.T) {
	logicObj := newAuthLogicForSession(nil, config.AuthConfig{SessionTTLSeconds: 7200})
	if got, want := logicObj.sessionTTL(), int64(3600); got != want {
		t.Fatalf("sessionTTL() = %d, want %d", got, want)
	}

	logicObj = newAuthLogicForSession(nil, config.AuthConfig{SessionTTLSeconds: 1200})
	if got, want := logicObj.sessionTTL(), int64(1200); got != want {
		t.Fatalf("sessionTTL() = %d, want %d", got, want)
	}
}

type failFirstDelRedis struct {
	redis.UniversalClient
	delCalls int
}

func (r *failFirstDelRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	r.delCalls++
	if r.delCalls == 1 {
		cmd := redis.NewIntCmd(ctx, keys)
		cmd.SetErr(stderrors.New("forced del failure"))
		return cmd
	}
	return r.UniversalClient.Del(ctx, keys...)
}

type failZAddRedis struct {
	redis.UniversalClient
}

func (r *failZAddRedis) ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx, "zadd", key)
	cmd.SetErr(stderrors.New("forced zadd failure"))
	return cmd
}

func newAuthLogicForSession(client redis.UniversalClient, authCfg config.AuthConfig) *AuthLogic {
	cfg := config.Config{
		AppID:        "site-a",
		JwtSecret:    "test-secret-please-change",
		JwtExpiresIn: 3600,
		Auth:         authCfg,
	}
	return NewAuthLogic(context.Background(), svc.NewServiceContext(cfg, "v1", svc.Dependencies{Rds: client}))
}

func sessionKeysForUser(keys []string, userID int64) []string {
	prefix := "api:user:session:site-a:" + strconv.FormatInt(userID, 10) + ":"
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			result = append(result, key)
		}
	}
	return result
}
