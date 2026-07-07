package auth

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	codes "api/common/codes"
	"api/common/idgen"
	"api/internal/config"
	"api/internal/infra/collectorx"
	userlogic "api/internal/logic/user"
	"api/internal/model"
	"api/internal/requestctx"
	"api/internal/routealias"
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

	registerCtx := authFlowContext(AuthEventActionRegisterSuccess, string(routealias.AuthRegister), http.MethodPost, "/api/auth/register", "10.0.0.8")
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

	loginCtx := authFlowContext(AuthEventActionLoginSuccess, string(routealias.AuthLogin), http.MethodPost, "/api/auth/login", "10.0.0.9")
	loginResp := requireAuthTokenResp(t, NewAuthLogic(loginCtx, svcCtx).Login(&types.LoginReq{
		Username: "demo_user",
		Password: "P@ssw0rd!",
	}), codes.Success)
	if loginResp.Token == registerResp.Token {
		t.Fatal("login token should differ from register token")
	}
	loginJTI := requireSessionToken(t, svcCtx, rds, loginResp.User.ID, loginResp.Token)
	requireSessionIndexMembers(t, svcCtx, rds, loginResp.User.ID, []string{registerJTI, loginJTI})

	user, err := model.FindUserByUsername(svcCtx.WriteDB(svc.DatabaseMain), "demo_user")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	if user == nil || user.LastLoginIP != "10.0.0.9" {
		t.Fatalf("user after login = %+v, want last_login_ip 10.0.0.9", user)
	}
	account, err := model.FindUserAccountByUsername(svcCtx.WriteDB(svc.DatabaseMain), "demo_user")
	if err != nil {
		t.Fatalf("FindUserAccountByUsername() error = %v", err)
	}
	if account == nil || account.UserID != user.ID || account.ShardNo != user.ShardNo || account.RouteShardCount != model.UserRouteShardCountDefault {
		t.Fatalf("user account = %+v, want user_id=%d shard_no=%d route=1", account, user.ID, user.ShardNo)
	}
	if err := model.UpdateUser(svcCtx.WriteDB(svc.DatabaseMain), user.ID, map[string]any{"email": "changed@example.com"}); err != nil {
		t.Fatalf("UpdateUser(email) error = %v", err)
	}

	profileCtx := authFlowAuthenticatedContext(string(routealias.UserProfile), http.MethodGet, "/api/user/profile", "10.0.0.9", loginResp)
	profile := requireUserProfile(t, userlogic.NewUserLogic(profileCtx, svcCtx).Profile(), codes.FetchSuccess)
	if profile.Email != "demo@example.com" {
		t.Fatalf("profile email = %q, want cached demo@example.com", profile.Email)
	}

	refreshCtx := authFlowAuthenticatedContext(string(routealias.AuthRefresh), http.MethodPost, "/api/auth/refresh", "10.0.0.9", loginResp)
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

	logoutCtx := authFlowAuthenticatedContext(string(routealias.AuthLogout), http.MethodPost, "/api/auth/logout", "10.0.0.9", refreshResp)
	logoutResult := NewAuthLogic(logoutCtx, svcCtx).Logout()
	if logoutResult == nil || !logoutResult.IsSuccess() || logoutResult.Code != codes.Success {
		t.Fatalf("Logout() = %+v, want success", logoutResult)
	}
	requireSessionMissing(t, svcCtx, rds, refreshResp.User.ID, refreshJTI)
	requireSessionIndexMembers(t, svcCtx, rds, refreshResp.User.ID, []string{registerJTI})

	requireAuthFlowEvents(t, *seen, []authFlowEventWant{
		{action: AuthEventActionRegisterSuccess, reason: AuthEventReasonSessionCreated, route: string(routealias.AuthRegister)},
		{action: AuthEventActionLoginSuccess, reason: AuthEventReasonSessionCreated, route: string(routealias.AuthLogin)},
		{action: AuthEventActionRefreshSuccess, reason: AuthEventReasonSessionRotated, route: string(routealias.AuthRefresh)},
		{action: AuthEventActionLogoutSuccess, reason: AuthEventReasonCurrentSessionDeleted, route: string(routealias.AuthLogout)},
	})
}

// authFlowEventWant 表示认证主流程期望采集到的风控事件关键字段。
type authFlowEventWant struct {
	action string // 期望事件动作
	reason string // 期望事件原因
	route  string // 期望路由别名
}

// newAuthFlowTestService 创建认证主流程测试所需的 SQLite、Redis 和采集器依赖。
func newAuthFlowTestService(t *testing.T) (*svc.ServiceContext, redis.UniversalClient, *[]collectorx.Event) {
	t.Helper()
	if err := idgen.ConfigureWorkerID(1); err != nil {
		t.Fatalf("ConfigureWorkerID() error = %v", err)
	}
	// SQLite 内存库按测试实例隔离，避免 go test -count 重复运行时复用旧数据。
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite) error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&authFlowUserSQLite{}, &authFlowUserAccountSQLite{}); err != nil {
		t.Fatalf("AutoMigrate(User) error = %v", err)
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
			Enabled: true,
		},
	}
	collector := &fakeCollector{events: make([]collectorx.Event, 0, 4)}
	svcCtx := svc.NewServiceContext(cfg, "v1", svc.Dependencies{
		SiteDBs: svc.SiteDatabases{MainDB: db},
		Rds:     client,
	})
	svcCtx.Collector = collector
	return svcCtx, client, &collector.events
}

// authFlowUserSQLite 使用 SQLite 创建用户表，业务读写仍走 model.User。
type authFlowUserSQLite struct {
	ID           int64     `gorm:"column:id;type:integer;primaryKey;autoIncrement:true;index:idx_user_shard_no_id,priority:2;index:idx_user_status_id,priority:2"` // 主键
	ShardNo      int       `gorm:"column:shard_no;type:int;not null;default:0;index:idx_user_shard_no_id,priority:1"`                                              // ID 哈希分片
	Username     string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_username"`                                                         // 用户名
	Nickname     string    `gorm:"column:nickname;type:varchar(64);not null;default:''"`                                                                           // 用户昵称
	PasswordHash string    `gorm:"column:password_hash;type:varchar(255);not null"`                                                                                // 密码哈希
	Email        string    `gorm:"column:email;type:varchar(128);not null;default:'';index:idx_user_email"`                                                        // 邮箱
	Phone        string    `gorm:"column:phone;type:varchar(32);not null;default:'';index:idx_user_phone"`                                                         // 手机号
	Avatar       string    `gorm:"column:avatar;type:varchar(255);not null;default:''"`                                                                            // 头像
	Status       int       `gorm:"column:status;type:tinyint;not null;default:1;index:idx_user_status_id,priority:1"`                                              // 用户状态
	LastLoginAt  time.Time `gorm:"column:last_login_at;type:datetime"`                                                                                             // 最后登录时间
	LastLoginIP  string    `gorm:"column:last_login_ip;type:varchar(45);not null;default:''"`                                                                      // 最后登录 IP
	CreatedAt    time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                             // 创建时间
	UpdatedAt    time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                             // 更新时间
}

// TableName 返回认证流程 SQLite 用户测试模型映射的真实表名。
func (*authFlowUserSQLite) TableName() string {
	return model.TableNameUser
}

// authFlowUserAccountSQLite 使用 SQLite 创建用户账号索引表，业务读写仍走 model.UserAccount。
type authFlowUserAccountSQLite struct {
	ID              uint64    `gorm:"column:id;type:integer;primaryKey;autoIncrement:true"`                                                                  // 主键
	Username        string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_account_username"`                                        // 用户名
	UserID          int64     `gorm:"column:user_id;type:integer;not null;uniqueIndex:uk_user_account_user_id;index:idx_user_account_shard_user,priority:2"` // 用户 ID
	ShardNo         int       `gorm:"column:shard_no;type:int;not null;index:idx_user_account_shard_user,priority:1"`                                        // 逻辑分片
	RouteShardCount int       `gorm:"column:route_shard_count;type:int;not null;default:1"`                                                                  // 物理表数量
	CreatedAt       time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                    // 创建时间
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                    // 更新时间
}

// TableName 返回认证流程 SQLite 账号索引测试模型映射的真实表名。
func (*authFlowUserAccountSQLite) TableName() string {
	return model.TableNameUserAccount
}

// authFlowContext 构造带路由、请求和链路信息的认证测试上下文。
func authFlowContext(action string, route string, method string, path string, clientIP string) context.Context {
	ctx, _ := requestctx.New(context.Background())
	requestctx.SetRoute(ctx, route)
	requestctx.SetRequest(ctx, method, path, clientIP)
	requestctx.SetTrace(ctx, "trace-"+action, "span-"+action)
	requestctx.SetNode(ctx, "node-a")
	requestctx.SetMode(ctx, "test")
	return ctx
}

// authFlowAuthenticatedContext 构造带当前用户和访问令牌的认证测试上下文。
func authFlowAuthenticatedContext(route string, method string, path string, clientIP string, tokenResp *types.AuthTokenResp) context.Context {
	ctx := authFlowContext(route, route, method, path, clientIP)
	if tokenResp != nil && tokenResp.User != nil {
		requestctx.SetUser(ctx, tokenResp.User.ID, tokenResp.User.Username, clientIP)
		requestctx.SetAccessToken(ctx, tokenResp.Token)
	}
	return ctx
}

// requireAuthTokenResp 断言业务结果为成功的认证令牌响应。
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

// requireUserProfile 断言业务结果为成功的用户资料响应。
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

// requireSessionToken 断言 Redis session token 存在并返回 token 中的 jti。
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

// requireSessionMissing 断言指定 jti 对应的 Redis session 已不存在。
func requireSessionMissing(t *testing.T, svcCtx *svc.ServiceContext, rds redis.UniversalClient, userID int64, jti string) {
	t.Helper()
	logicObj := NewAuthLogic(context.Background(), svcCtx)
	err := rds.Get(context.Background(), logicObj.userSessionKey(userID, jti)).Err()
	if !stderrors.Is(err, redis.Nil) {
		t.Fatalf("session %s err = %v, want redis.Nil", jti, err)
	}
}

// requireSessionIndexMembers 断言用户 session 索引中仅包含期望的 jti 集合。
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

// requireAuthFlowEvents 断言认证流程采集事件顺序、字段和脱敏结果。
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

// sameStringSet 判断两个字符串切片是否包含相同元素集合。
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
