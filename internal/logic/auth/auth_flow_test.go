package auth

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strings"
	"testing"
	"time"

	codes "api/common/codes"
	"api/internal/config"
	"api/internal/infra/collectorx"
	userlogic "api/internal/logic/user"
	"api/internal/model"
	"api/internal/requestctx"
	"api/internal/svc"
	"api/internal/types"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestAuthMainFlowIntegration 覆盖前台认证主链路的 DB、Redis session、资料缓存和风控事件。
func TestAuthMainFlowIntegration(t *testing.T) {
	svcCtx, rds, seen := newAuthFlowTestService(t)

	registerCtx := authFlowContext(AuthEventActionRegisterSuccess, "auth.register", http.MethodPost, "/api/auth/register", "10.0.0.8")
	registerResp := requireAuthTokenResp(t, NewAuthLogic(registerCtx, svcCtx).Register(&types.RegisterReq{
		Username: "demo_user",
		Password: "P@ssw0rd!",
		Nickname: "Demo",
		Email:    "demo@example.com",
		Phone:    "13800000000",
	}), codes.CreateSuccess)
	if registerResp.User == nil || registerResp.User.ID <= 0 || registerResp.User.Email != "demo@example.com" {
		t.Fatalf("register user = %+v, want created profile", registerResp.User)
	}
	registerJTI := requireSessionToken(t, svcCtx, rds, registerResp.User.ID, registerResp.Token)
	requireSessionIndexMembers(t, svcCtx, rds, registerResp.User.ID, []string{registerJTI})

	loginCtx := authFlowContext(AuthEventActionLoginSuccess, "auth.login", http.MethodPost, "/api/auth/login", "10.0.0.9")
	loginResp := requireAuthTokenResp(t, NewAuthLogic(loginCtx, svcCtx).Login(&types.LoginReq{
		Username: "demo_user",
		Password: "P@ssw0rd!",
	}), codes.Success)
	if loginResp.Token == registerResp.Token {
		t.Fatal("login token should differ from register token")
	}
	loginJTI := requireSessionToken(t, svcCtx, rds, loginResp.User.ID, loginResp.Token)
	requireSessionIndexMembers(t, svcCtx, rds, loginResp.User.ID, []string{registerJTI, loginJTI})

	user, err := model.FindAPIUserByUsername(svcCtx.WriteDB(svc.DatabaseMain), "demo_user")
	if err != nil {
		t.Fatalf("FindAPIUserByUsername() error = %v", err)
	}
	if user == nil || user.LastLoginIP != "10.0.0.9" {
		t.Fatalf("user after login = %+v, want last_login_ip 10.0.0.9", user)
	}
	if err := model.UpdateAPIUser(svcCtx.WriteDB(svc.DatabaseMain), user.ID, map[string]any{"email": "changed@example.com"}); err != nil {
		t.Fatalf("UpdateAPIUser(email) error = %v", err)
	}

	profileCtx := authFlowAuthenticatedContext("user.profile", http.MethodGet, "/api/user/profile", "10.0.0.9", loginResp)
	profile := requireUserProfile(t, userlogic.NewUserLogic(profileCtx, svcCtx).Profile(), codes.FetchSuccess)
	if profile.Email != "demo@example.com" {
		t.Fatalf("profile email = %q, want cached demo@example.com", profile.Email)
	}

	refreshCtx := authFlowAuthenticatedContext("auth.refresh", http.MethodPost, "/api/auth/refresh", "10.0.0.9", loginResp)
	refreshResp := requireAuthTokenResp(t, NewAuthLogic(refreshCtx, svcCtx).Refresh(), codes.Success)
	if refreshResp.Token == loginResp.Token {
		t.Fatal("refresh token should differ from login token")
	}
	if refreshResp.User == nil || refreshResp.User.Email != "changed@example.com" {
		t.Fatalf("refresh user = %+v, want latest primary DB profile", refreshResp.User)
	}
	refreshJTI := requireSessionToken(t, svcCtx, rds, refreshResp.User.ID, refreshResp.Token)
	requireSessionMissing(t, svcCtx, rds, refreshResp.User.ID, loginJTI)
	requireSessionIndexMembers(t, svcCtx, rds, refreshResp.User.ID, []string{registerJTI, refreshJTI})

	logoutCtx := authFlowAuthenticatedContext("auth.logout", http.MethodPost, "/api/auth/logout", "10.0.0.9", refreshResp)
	logoutResult := NewAuthLogic(logoutCtx, svcCtx).Logout()
	if logoutResult == nil || !logoutResult.IsSuccess() || logoutResult.Code != codes.Success {
		t.Fatalf("Logout() = %+v, want success", logoutResult)
	}
	requireSessionMissing(t, svcCtx, rds, refreshResp.User.ID, refreshJTI)
	requireSessionIndexMembers(t, svcCtx, rds, refreshResp.User.ID, []string{registerJTI})

	requireAuthFlowEvents(t, *seen, []authFlowEventWant{
		{action: AuthEventActionRegisterSuccess, reason: AuthEventReasonSessionCreated, route: "auth.register"},
		{action: AuthEventActionLoginSuccess, reason: AuthEventReasonSessionCreated, route: "auth.login"},
		{action: AuthEventActionRefreshSuccess, reason: AuthEventReasonSessionRotated, route: "auth.refresh"},
		{action: AuthEventActionLogoutSuccess, reason: AuthEventReasonCurrentSessionDeleted, route: "auth.logout"},
	})
}

type authFlowEventWant struct {
	action string // 期望事件动作
	reason string // 期望事件原因
	route  string // 期望路由别名
}

func newAuthFlowTestService(t *testing.T) (*svc.ServiceContext, redis.UniversalClient, *[]collectorx.Event) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	if err := db.AutoMigrate(&authFlowAPIUserSQLite{}); err != nil {
		t.Fatalf("AutoMigrate(APIUser) error = %v", err)
	}

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	cfg := config.Config{
		AppID:        "site-a",
		AppKey:       "event-secret",
		JwtSecret:    "test-secret-please-change",
		JwtExpiresIn: 3600,
		Auth: config.AuthConfig{
			RegisterEnabled:        true,
			SessionTTLSeconds:      1200,
			ProfileCacheTTLSeconds: 300,
			PasswordMinLength:      8,
		},
		Collector: config.CollectorConfig{
			Enabled:   true,
			Transport: "sync",
		},
	}
	manager, err := collectorx.New(cfg.Collector, client)
	if err != nil {
		t.Fatalf("collectorx.New() error = %v", err)
	}
	seen := make([]collectorx.Event, 0, 4)
	if err := manager.RegisterProcessorFunc(AuthCollectorBizType, func(ctx context.Context, events []collectorx.Event) ([]collectorx.ProcessResult, error) {
		seen = append(seen, events...)
		results := make([]collectorx.ProcessResult, 0, len(events))
		for _, event := range events {
			results = append(results, collectorx.ProcessResult{EventID: event.EventID, Success: true})
		}
		return results, nil
	}); err != nil {
		t.Fatalf("RegisterProcessorFunc() error = %v", err)
	}
	svcCtx := svc.NewServiceContext(cfg, "v1", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{MainDB: db},
		Rds:     client,
	})
	svcCtx.Collector = manager
	return svcCtx, client, &seen
}

// authFlowAPIUserSQLite 使用 SQLite 自增主键创建测试表，业务读写仍走 model.APIUser。
type authFlowAPIUserSQLite struct {
	ID           int64     `gorm:"column:id;type:integer;primaryKey;autoIncrement:true"` // 主键
	Username     string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_api_user_username"`
	Nickname     string    `gorm:"column:nickname;type:varchar(64);not null;default:''"`
	PasswordHash string    `gorm:"column:password_hash;type:varchar(255);not null"`
	Email        string    `gorm:"column:email;type:varchar(128);not null;default:'';index:idx_api_user_email"`
	Phone        string    `gorm:"column:phone;type:varchar(32);not null;default:'';index:idx_api_user_phone"`
	Avatar       string    `gorm:"column:avatar;type:varchar(255);not null;default:''"`
	Status       int       `gorm:"column:status;type:tinyint;not null;default:1;index:idx_api_user_status"`
	LastLoginAt  time.Time `gorm:"column:last_login_at;type:timestamp"`
	LastLoginIP  string    `gorm:"column:last_login_ip;type:varchar(45);not null;default:''"`
	CreatedAt    time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

func (*authFlowAPIUserSQLite) TableName() string {
	return model.TableNameAPIUser
}

func authFlowContext(action string, route string, method string, path string, clientIP string) context.Context {
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetRoute(ctx, route)
	requestctx.SetRequest(ctx, method, path, clientIP)
	requestctx.SetTrace(ctx, "trace-"+action, "span-"+action)
	requestctx.SetNode(ctx, "node-a")
	requestctx.SetMode(ctx, "test")
	return ctx
}

func authFlowAuthenticatedContext(route string, method string, path string, clientIP string, tokenResp *types.AuthTokenResp) context.Context {
	ctx := authFlowContext(route, route, method, path, clientIP)
	if tokenResp != nil && tokenResp.User != nil {
		requestctx.SetUser(ctx, tokenResp.User.ID, tokenResp.User.Username, clientIP)
		requestctx.SetAccessToken(ctx, tokenResp.Token)
	}
	return ctx
}

func requireAuthTokenResp(t *testing.T, result *types.BizResult, code int) *types.AuthTokenResp {
	t.Helper()
	if result == nil || !result.IsSuccess() || result.Code != code {
		t.Fatalf("auth result = %+v, want success code=%d", result, code)
	}
	resp, ok := result.Data.(*types.AuthTokenResp)
	if !ok || resp == nil || strings.TrimSpace(resp.Token) == "" || resp.ExpiresAt <= 0 || resp.User == nil {
		t.Fatalf("auth result data = %#v, want AuthTokenResp", result.Data)
	}
	return resp
}

func requireUserProfile(t *testing.T, result *types.BizResult, code int) *types.UserProfile {
	t.Helper()
	if result == nil || !result.IsSuccess() || result.Code != code {
		t.Fatalf("profile result = %+v, want success code=%d", result, code)
	}
	profile, ok := result.Data.(*types.UserProfile)
	if !ok || profile == nil || profile.ID <= 0 {
		t.Fatalf("profile result data = %#v, want UserProfile", result.Data)
	}
	return profile
}

func requireSessionToken(t *testing.T, svcCtx *svc.ServiceContext, rds redis.UniversalClient, userID int64, token string) string {
	t.Helper()
	jti := tokenJTI(token, svcCtx.CurrentConfig().JwtSecret)
	if jti == "" {
		t.Fatal("token jti is empty")
	}
	logicObj := NewAuthLogic(context.Background(), svcCtx)
	got, err := rds.Get(context.Background(), logicObj.userSessionKey(userID, jti)).Result()
	if err != nil {
		t.Fatalf("Get(session %s) error = %v", jti, err)
	}
	if got != token {
		t.Fatalf("session token mismatch jti=%s", jti)
	}
	return jti
}

func requireSessionMissing(t *testing.T, svcCtx *svc.ServiceContext, rds redis.UniversalClient, userID int64, jti string) {
	t.Helper()
	logicObj := NewAuthLogic(context.Background(), svcCtx)
	err := rds.Get(context.Background(), logicObj.userSessionKey(userID, jti)).Err()
	if !stderrors.Is(err, redis.Nil) {
		t.Fatalf("session %s err = %v, want redis.Nil", jti, err)
	}
}

func requireSessionIndexMembers(t *testing.T, svcCtx *svc.ServiceContext, rds redis.UniversalClient, userID int64, want []string) {
	t.Helper()
	logicObj := NewAuthLogic(context.Background(), svcCtx)
	got, err := rds.ZRange(context.Background(), logicObj.userSessionIndexKey(userID), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange(session index) error = %v", err)
	}
	if !sameStringSet(got, want) {
		t.Fatalf("session index = %v, want %v", got, want)
	}
}

func requireAuthFlowEvents(t *testing.T, events []collectorx.Event, wants []authFlowEventWant) {
	t.Helper()
	if len(events) != len(wants) {
		t.Fatalf("auth events = %d, want %d", len(events), len(wants))
	}
	for index, want := range wants {
		var payload authEventPayload
		if err := json.Unmarshal(events[index].Payload, &payload); err != nil {
			t.Fatalf("Unmarshal(event[%d]) error = %v", index, err)
		}
		if payload.Action != want.action || payload.Reason != want.reason || payload.Route != want.route {
			t.Fatalf("event[%d] payload = %+v, want action=%s reason=%s route=%s", index, payload, want.action, want.reason, want.route)
		}
		raw := string(events[index].Payload)
		for _, forbidden := range []string{"demo_user", "P@ssw0rd!", "10.0.0."} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("event[%d] leaked raw value %q: %s", index, forbidden, raw)
			}
		}
	}
}

func sameStringSet(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(want))
	for _, item := range got {
		counts[item]++
	}
	for _, item := range want {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
