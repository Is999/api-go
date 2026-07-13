package auth

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	keys "api/common/rediskeys"
	"api/internal/config"
	"api/internal/model"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/redis/go-redis/v9"
)

// TestGenerateJWTUsesStringSnowflakeSubjectAndAuthVersion 确保 JWT 无损承载用户 ID 和认证版本。
func TestGenerateJWTUsesStringSnowflakeSubjectAndAuthVersion(t *testing.T) {
	const userID int64 = 9_007_199_254_740_993
	logicObj := newAuthLogicForSession(nil, config.AuthConfig{SessionTTLSeconds: 60})
	tokenString, _, err := logicObj.generateJWT(&model.User{ID: userID, Username: "demo", AuthVersion: 7}, "test-sid", "test-jti")
	if err != nil {
		t.Fatalf("generateJWT() error = %v", err)
	}
	claims := jwt.MapClaims{}
	if _, _, err = jwt.NewParser().ParseUnverified(tokenString, claims); err != nil {
		t.Fatalf("ParseUnverified() error = %v", err)
	}
	if claims["sub"] != strconv.FormatInt(userID, 10) {
		t.Fatalf("claims[sub] = %#v, want %q", claims["sub"], strconv.FormatInt(userID, 10))
	}
	if claims["auth_version"] != float64(7) {
		t.Fatalf("claims[auth_version] = %#v, want 7", claims["auth_version"])
	}
	if claims["sid"] != "test-sid" || claims["jti"] != "test-jti" {
		t.Fatalf("session claims = sid:%#v jti:%#v", claims["sid"], claims["jti"])
	}
}

// TestCreateSessionWritesAtomicState 确保创建会话同步写入 Hash、索引和认证版本。
func TestCreateSessionWritesAtomicState(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	created, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	if got := client.HGet(context.Background(), keys.UserSessionHashKey(user.ID), created.SessionID).Val(); got != created.Response.Token {
		t.Fatalf("session token = %q, want created token", got)
	}
	if members := client.ZRange(context.Background(), keys.UserSessionIndexKey(user.ID), 0, -1).Val(); len(members) != 1 || members[0] != created.SessionID {
		t.Fatalf("index members = %v, want [%s]", members, created.SessionID)
	}
	if version := client.Get(context.Background(), keys.UserSessionAuthVersionKey(user.ID)).Val(); version != "1" {
		t.Fatalf("auth version = %q, want 1", version)
	}
	hashTTL := client.TTL(context.Background(), keys.UserSessionHashKey(user.ID)).Val()
	if hashTTL <= 0 {
		t.Fatalf("session hash ttl = %v, want positive", hashTTL)
	}
	versionTTL := client.TTL(context.Background(), keys.UserSessionAuthVersionKey(user.ID)).Val()
	if versionTTL < hashTTL-time.Second || versionTTL > hashTTL+time.Second {
		t.Fatalf("auth version ttl = %v, want synchronized with session ttl %v", versionTTL, hashTTL)
	}
}

// TestCreateSessionKeepsLongestTTL 确保新短会话不会缩短仍有效旧会话的容器 TTL。
func TestCreateSessionKeepsLongestTTL(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 3600)
	user := sessionTestUser(1)
	if _, err := logicObj.createSession(user); err != nil {
		t.Fatalf("createSession(first) error = %v", err)
	}
	firstTTL := client.TTL(context.Background(), keys.UserSessionHashKey(user.ID)).Val()

	cfg := logicObj.Svc.CurrentConfig()
	cfg.Auth.SessionTTLSeconds = 60
	logicObj.Svc.UpdateConfig(cfg)
	if _, err := logicObj.createSession(user); err != nil {
		t.Fatalf("createSession(second) error = %v", err)
	}
	if remainingTTL := client.TTL(context.Background(), keys.UserSessionHashKey(user.ID)).Val(); remainingTTL < firstTTL-time.Second {
		t.Fatalf("short session reduced hash ttl: before=%v after=%v", firstTTL, remainingTTL)
	}
}

// TestCreateSessionPrunesExpiredAndCapsSessions 确保创建时清理过期会话并把每用户有效会话限制在硬上限内。
func TestCreateSessionPrunesExpiredAndCapsSessions(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 3600)
	user := sessionTestUser(1)
	ctx := context.Background()
	if err := client.HSet(ctx, keys.UserSessionHashKey(user.ID), "expired", "expired-token").Err(); err != nil {
		t.Fatalf("HSet(expired) error = %v", err)
	}
	if err := client.ZAdd(ctx, keys.UserSessionIndexKey(user.ID), redis.Z{Score: float64(time.Now().Add(-time.Minute).UnixMilli()), Member: "expired"}).Err(); err != nil {
		t.Fatalf("ZAdd(expired) error = %v", err)
	}

	sessionIDs := make([]string, 0, maxUserSessions+2)
	for index := 0; index < maxUserSessions+2; index++ {
		created, err := logicObj.createSession(user)
		if err != nil {
			t.Fatalf("createSession(%d) error = %v", index, err)
		}
		sessionIDs = append(sessionIDs, created.SessionID)
		time.Sleep(time.Millisecond)
	}
	if client.HExists(ctx, keys.UserSessionHashKey(user.ID), "expired").Val() {
		t.Fatal("expired session still exists")
	}
	if count := client.HLen(ctx, keys.UserSessionHashKey(user.ID)).Val(); count != maxUserSessions {
		t.Fatalf("session count = %d, want %d", count, maxUserSessions)
	}
	for _, evicted := range sessionIDs[:2] {
		if client.HExists(ctx, keys.UserSessionHashKey(user.ID), evicted).Val() {
			t.Fatalf("oldest session %s still exists", evicted)
		}
	}
}

// TestCreateSessionAdvancesVersionAndRejectsStaleSnapshot 确保新数据库版本原子清旧会话，旧快照不能覆盖新版本。
func TestCreateSessionAdvancesVersionAndRejectsStaleSnapshot(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	oldUser := sessionTestUser(1)
	oldSession, err := logicObj.createSession(oldUser)
	if err != nil {
		t.Fatalf("create old session error = %v", err)
	}
	newUser := sessionTestUser(2)
	newSession, err := logicObj.createSession(newUser)
	if err != nil {
		t.Fatalf("create new version session error = %v", err)
	}
	ctx := context.Background()
	if client.HExists(ctx, keys.UserSessionHashKey(oldUser.ID), oldSession.SessionID).Val() {
		t.Fatal("old auth version session still exists")
	}
	if !client.HExists(ctx, keys.UserSessionHashKey(newUser.ID), newSession.SessionID).Val() {
		t.Fatal("new auth version session missing")
	}
	if _, err = logicObj.createSession(oldUser); !errors.Is(err, ErrAuthVersionMismatch) {
		t.Fatalf("stale create error = %v, want ErrAuthVersionMismatch", err)
	}
}

// TestSessionAuthVersionKeepsUint64Precision 确保 Lua 不把 uint64 认证版本转换为双精度浮点数。
func TestSessionAuthVersionKeepsUint64Precision(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(^uint64(0))
	if _, err := logicObj.createSession(user); err != nil {
		t.Fatalf("createSession(max uint64) error = %v", err)
	}
	if version := client.Get(context.Background(), keys.UserSessionAuthVersionKey(user.ID)).Val(); version != "18446744073709551615" {
		t.Fatalf("auth version = %q, want max uint64", version)
	}
	if err := logicObj.InvalidateUserSessions(user.ID, ^uint64(0)-1); !errors.Is(err, ErrAuthVersionMismatch) {
		t.Fatalf("stale max uint64 invalidate error = %v, want ErrAuthVersionMismatch", err)
	}
}

// TestCreateAndInvalidateVersionIsolation 确保旧数据库快照与新版本失效并发时不会留下逃逸会话。
func TestCreateAndInvalidateVersionIsolation(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	oldUser := sessionTestUser(1)
	ctx := context.Background()
	for round := 0; round < 50; round++ {
		if err := client.Del(ctx, keys.UserSessionKeys(oldUser.ID)...).Err(); err != nil {
			t.Fatalf("Del(session state) error = %v", err)
		}
		if _, err := logicObj.createSession(oldUser); err != nil {
			t.Fatalf("create initial session error = %v", err)
		}

		start := make(chan struct{})
		var createErr error
		var invalidateErr error
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			_, createErr = logicObj.createSession(oldUser)
		}()
		go func() {
			defer workers.Done()
			<-start
			invalidateErr = logicObj.InvalidateUserSessions(oldUser.ID, 2)
		}()
		close(start)
		workers.Wait()
		if createErr != nil && !errors.Is(createErr, ErrAuthVersionMismatch) {
			t.Fatalf("round %d create error = %v", round, createErr)
		}
		if invalidateErr != nil {
			t.Fatalf("round %d invalidate error = %v", round, invalidateErr)
		}
		if version := client.Get(ctx, keys.UserSessionAuthVersionKey(oldUser.ID)).Val(); version != "2" {
			t.Fatalf("round %d auth version = %q, want 2", round, version)
		}
		if count := client.HLen(ctx, keys.UserSessionHashKey(oldUser.ID)).Val(); count != 0 {
			t.Fatalf("round %d stale session count = %d, want 0", round, count)
		}
	}
}

// TestRotateSessionConsumesPreviousTokenOnce 确保并发刷新同一旧 token 时最多一个调用成功。
func TestRotateSessionConsumesPreviousTokenOnce(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	previous, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	first, err := logicObj.rotateSession(user, previous.SessionID, previous.Response.Token)
	if err != nil {
		t.Fatalf("rotateSession(first) error = %v", err)
	}
	if _, err = logicObj.rotateSession(user, previous.SessionID, previous.Response.Token); !errors.Is(err, ErrSessionStale) {
		t.Fatalf("rotateSession(second) error = %v, want ErrSessionStale", err)
	}
	_, previousJTI := tokenSessionClaimsForTest(previous.Response.Token, logicObj.Svc.CurrentConfig().JwtSecret)
	newSID, newJTI := tokenSessionClaimsForTest(first.Token, logicObj.Svc.CurrentConfig().JwtSecret)
	if newSID != previous.SessionID || newJTI == previousJTI {
		t.Fatalf("rotated claims sid=%q jti=%q, want sid=%q and a new jti", newSID, newJTI, previous.SessionID)
	}
	members := client.ZRange(context.Background(), keys.UserSessionIndexKey(user.ID), 0, -1).Val()
	if len(members) != 1 || members[0] != previous.SessionID {
		t.Fatalf("index members = %v, want [%s]", members, previous.SessionID)
	}
}

// TestRotateSessionConcurrentCAS 确保多个实例并发刷新同一旧 token 时只有一个 Lua CAS 成功。
func TestRotateSessionConcurrentCAS(t *testing.T) {
	logicObj, _ := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	previous, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}

	var succeeded atomic.Int64
	var stale atomic.Int64
	var unexpected atomic.Int64
	var workers sync.WaitGroup
	for index := 0; index < 16; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, rotateErr := logicObj.rotateSession(user, previous.SessionID, previous.Response.Token)
			switch {
			case rotateErr == nil:
				succeeded.Add(1)
			case errors.Is(rotateErr, ErrSessionStale):
				stale.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	workers.Wait()
	if succeeded.Load() != 1 || stale.Load() != 15 || unexpected.Load() != 0 {
		t.Fatalf("refresh results success=%d stale=%d unexpected=%d, want 1/15/0", succeeded.Load(), stale.Load(), unexpected.Load())
	}
}

// TestRefreshThenLogoutDeletesRotatedSession 确保刷新成功后退出仍按稳定 sid 删除新 token。
func TestRefreshThenLogoutDeletesRotatedSession(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	created, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if _, err = logicObj.rotateSession(user, created.SessionID, created.Response.Token); err != nil {
		t.Fatalf("rotateSession() error = %v", err)
	}
	if err = logicObj.deleteUserSession(user.ID, created.SessionID); err != nil {
		t.Fatalf("deleteUserSession() error = %v", err)
	}
	requireNoSessionState(t, client, user.ID)
	if ttl := client.TTL(context.Background(), keys.UserSessionAuthVersionKey(user.ID)).Val(); ttl <= 0 {
		t.Fatalf("auth version fence ttl after logout = %v, want positive", ttl)
	}
}

// TestLogoutThenRefreshRejectsDeletedSession 确保退出先完成时刷新完整旧 token 的 CAS 失败。
func TestLogoutThenRefreshRejectsDeletedSession(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	created, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if err = logicObj.deleteUserSession(user.ID, created.SessionID); err != nil {
		t.Fatalf("deleteUserSession() error = %v", err)
	}
	if _, err = logicObj.rotateSession(user, created.SessionID, created.Response.Token); !errors.Is(err, ErrSessionStale) {
		t.Fatalf("rotateSession() error = %v, want ErrSessionStale", err)
	}
	requireNoSessionState(t, client, user.ID)
}

// TestRefreshLogoutConcurrentLeavesNoSession 确保刷新与退出真实并发后不会留下逃逸会话。
func TestRefreshLogoutConcurrentLeavesNoSession(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	for round := 0; round < 50; round++ {
		created, err := logicObj.createSession(user)
		if err != nil {
			t.Fatalf("round %d createSession() error = %v", round, err)
		}

		start := make(chan struct{})
		var rotateErr error
		var deleteErr error
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			_, rotateErr = logicObj.rotateSession(user, created.SessionID, created.Response.Token)
		}()
		go func() {
			defer workers.Done()
			<-start
			deleteErr = logicObj.deleteUserSession(user.ID, created.SessionID)
		}()
		close(start)
		workers.Wait()

		if rotateErr != nil && !errors.Is(rotateErr, ErrSessionStale) {
			t.Fatalf("round %d rotate error = %v", round, rotateErr)
		}
		if deleteErr != nil {
			t.Fatalf("round %d delete error = %v", round, deleteErr)
		}
		requireNoSessionState(t, client, user.ID)
	}
}

// TestDeleteUserSessionRemovesHashAndIndex 确保退出登录原子删除 Hash 字段和索引成员。
func TestDeleteUserSessionRemovesHashAndIndex(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	created, err := logicObj.createSession(user)
	if err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	if err := logicObj.deleteUserSession(user.ID, created.SessionID); err != nil {
		t.Fatalf("deleteUserSession() error = %v", err)
	}
	ctx := context.Background()
	if client.HExists(ctx, keys.UserSessionHashKey(user.ID), created.SessionID).Val() {
		t.Fatal("deleted session still exists")
	}
	if members := client.ZRange(ctx, keys.UserSessionIndexKey(user.ID), 0, -1).Val(); len(members) != 0 {
		t.Fatalf("index members = %v, want empty", members)
	}
}

// TestDiscardRegistrationRuntimeStateIgnoresCanceledRequest 确保注册回滚补偿不受请求取消影响。
func TestDiscardRegistrationRuntimeStateIgnoresCanceledRequest(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	if _, err := logicObj.createSession(user); err != nil {
		t.Fatalf("createSession() error = %v", err)
	}
	profileKey := logicObj.AppRedisKey(fmt.Sprintf(keys.UserProfile, user.ID))
	if exists := client.Exists(context.Background(), profileKey).Val(); exists != 1 {
		t.Fatalf("profile cache exists = %d, want 1", exists)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledLogic := NewAuthLogic(canceledCtx, logicObj.Svc)
	if err := canceledLogic.discardRegistrationRuntimeState(user.ID); err != nil {
		t.Fatalf("discardRegistrationRuntimeState() error = %v", err)
	}
	keysToCheck := append(keys.UserSessionKeys(user.ID), profileKey)
	if exists := client.Exists(context.Background(), keysToCheck...).Val(); exists != 0 {
		t.Fatalf("registration runtime keys still exist = %d, keys=%v", exists, keysToCheck)
	}
}

// TestInvalidateUserSessionsUsesCommittedVersion 确保全量失效原子清理会话并拒绝旧版本覆盖。
func TestInvalidateUserSessionsUsesCommittedVersion(t *testing.T) {
	logicObj, client := newAuthSessionTest(t, 60)
	user := sessionTestUser(1)
	for index := 0; index < 2; index++ {
		if _, err := logicObj.createSession(user); err != nil {
			t.Fatalf("createSession(%d) error = %v", index, err)
		}
	}
	if err := logicObj.InvalidateUserSessions(user.ID, 2); err != nil {
		t.Fatalf("InvalidateUserSessions() error = %v", err)
	}
	ctx := context.Background()
	if count := client.HLen(ctx, keys.UserSessionHashKey(user.ID)).Val(); count != 0 {
		t.Fatalf("session count = %d, want 0", count)
	}
	if version := client.Get(ctx, keys.UserSessionAuthVersionKey(user.ID)).Val(); version != "2" {
		t.Fatalf("auth version = %q, want 2", version)
	}
	versionTTL := client.TTL(ctx, keys.UserSessionAuthVersionKey(user.ID)).Val()
	if want := time.Duration(logicObj.authVersionFenceTTL()) * time.Second; versionTTL < want-time.Second || versionTTL > want {
		t.Fatalf("auth version ttl = %v, want %v", versionTTL, want)
	}
	if err := logicObj.InvalidateUserSessions(user.ID, 1); !errors.Is(err, ErrAuthVersionMismatch) {
		t.Fatalf("stale invalidate error = %v, want ErrAuthVersionMismatch", err)
	}
}

// TestSessionKeysShareClusterHashTag 确保会话 Lua 使用的全部 Key 可在 Redis Cluster 同槽执行。
func TestSessionKeysShareClusterHashTag(t *testing.T) {
	got := keys.UserSessionKeys(42)
	want := []string{
		"app:site-a:user:session:{42}",
		"app:site-a:user:session:index:{42}",
		"app:site-a:user:session:auth_version:{42}",
	}
	sort.Strings(got)
	sort.Strings(want)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("session key[%d] = %q, want %q", index, got[index], want[index])
		}
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

// requireNoSessionState 断言会话 Hash 和 sid 索引均为空。
func requireNoSessionState(t *testing.T, client redis.UniversalClient, userID int64) {
	t.Helper()
	ctx := context.Background()
	if count := client.HLen(ctx, keys.UserSessionHashKey(userID)).Val(); count != 0 {
		t.Fatalf("session hash count = %d, want 0", count)
	}
	if count := client.ZCard(ctx, keys.UserSessionIndexKey(userID)).Val(); count != 0 {
		t.Fatalf("session index count = %d, want 0", count)
	}
}

// newAuthSessionTest 构造带 miniredis 的会话测试依赖。
func newAuthSessionTest(t *testing.T, ttlSeconds int64) (*AuthLogic, redis.UniversalClient) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return newAuthLogicForSession(client, config.AuthConfig{SessionTTLSeconds: ttlSeconds}), client
}

// newAuthLogicForSession 构造认证逻辑测试依赖。
func newAuthLogicForSession(client redis.UniversalClient, authCfg config.AuthConfig) *AuthLogic {
	cfg := config.Config{
		AppID:        "site-a",
		JwtSecret:    "test-secret-please-change",
		JwtExpiresIn: 3600,
		Auth:         authCfg,
	}
	return NewAuthLogic(context.Background(), svc.NewServiceContext(cfg, "v1", svc.Dependencies{Rds: client}))
}

// sessionTestUser 返回指定认证版本的测试用户。
func sessionTestUser(authVersion uint64) *model.User {
	return &model.User{ID: 42, Username: "demo", Status: model.UserStatusEnabled, AuthVersion: authVersion}
}

// tokenSessionClaimsForTest 解析测试 token 中的 sid 和 jti。
func tokenSessionClaimsForTest(tokenString string, secret string) (string, string) {
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	token, err := parser.ParseWithClaims(strings.TrimSpace(tokenString), claims, func(*jwt.Token) (interface{}, error) {
		return []byte(strings.TrimSpace(secret)), nil
	})
	if err != nil || token == nil || !token.Valid {
		return "", ""
	}
	sessionID, _ := claims["sid"].(string)
	jti, _ := claims["jti"].(string)
	return strings.TrimSpace(sessionID), strings.TrimSpace(jti)
}
