package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	codes "api/common/codes"
	"api/common/idgen"
	keys "api/common/rediskeys"
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
	if registerResp.User == nil || registerResp.User.ID <= 0 || registerResp.User.Email != "de***o@example.com" || registerResp.User.Phone != "138****0000" {
		t.Fatalf("register user = %+v, want created profile", registerResp.User)
	}
	registerSID := requireSessionToken(t, svcCtx, rds, registerResp.User.ID, registerResp.Token)
	requireSessionIndexMembers(t, rds, registerResp.User.ID, []string{registerSID})

	loginCtx := authFlowContext(AuthEventActionLoginSuccess, string(routealias.AuthLogin), http.MethodPost, "/api/auth/login", "10.0.0.9")
	loginResp := requireAuthTokenResp(t, NewAuthLogic(loginCtx, svcCtx).Login(&types.LoginReq{
		IdentityType:  types.LoginIdentityTypeEmail,
		IdentityValue: "demo@example.com",
		Password:      "P@ssw0rd!",
	}), codes.Success)
	if loginResp.Token == registerResp.Token {
		t.Fatal("login token should differ from register token")
	}
	loginSID := requireSessionToken(t, svcCtx, rds, loginResp.User.ID, loginResp.Token)
	requireSessionIndexMembers(t, rds, loginResp.User.ID, []string{registerSID, loginSID})

	cfg := svcCtx.CurrentConfig()
	user, err := model.FindUserByIdentity(svcCtx.WriteDB(svc.DatabaseMain), model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, "demo_user", cfg.AppKey, cfg.User.RouteShardCount)
	if err != nil {
		t.Fatalf("FindUserByIdentity(username) error = %v", err)
	}
	if user == nil || user.LastLoginIP != "10.0.0.9" {
		t.Fatalf("user after login = %+v, want last_login_ip 10.0.0.9", user)
	}
	identity, err := model.FindUserIdentity(svcCtx.WriteDB(svc.DatabaseMain), model.UserIdentityTypePhone, model.UserIdentityProviderLocal, "13800000000", svcCtx.CurrentConfig().AppKey)
	if err != nil {
		t.Fatalf("FindUserIdentity(phone) error = %v", err)
	}
	if identity == nil || identity.UserID != user.ID || identity.UserShardNo != user.ShardNo {
		t.Fatalf("user identity = %+v, want user_id=%d user_shard_no=%d", identity, user.ID, user.ShardNo)
	}
	if err := model.UpdateUserProfileWithIdentities(svcCtx.WriteDB(svc.DatabaseMain), user.ID, map[string]any{"email": "changed@example.com"}, cfg.AppKey, cfg.User.RouteShardCount); err != nil {
		t.Fatalf("UpdateUserProfileWithIdentities(email) error = %v", err)
	}

	profileCtx := authFlowAuthenticatedContext(string(routealias.UserProfile), http.MethodGet, "/api/user/profile", "10.0.0.9", loginResp)
	profile := requireUserProfile(t, userlogic.NewUserLogic(profileCtx, svcCtx).Profile(), codes.FetchSuccess)
	if profile.Email != "de***o@example.com" {
		t.Fatalf("profile email = %q, want cached de***o@example.com", profile.Email)
	}

	refreshCtx := authFlowAuthenticatedContext(string(routealias.AuthRefresh), http.MethodPost, "/api/auth/refresh", "10.0.0.9", loginResp)
	refreshResp := requireAuthTokenResp(t, NewAuthLogic(refreshCtx, svcCtx).Refresh(), codes.Success)
	if refreshResp.Token == loginResp.Token {
		t.Fatal("refresh token should differ from login token")
	}
	if refreshResp.User == nil || refreshResp.User.Email != "ch****d@example.com" {
		t.Fatalf("refresh user = %+v, want latest primary DB profile", refreshResp.User)
	}
	refreshSID := requireSessionToken(t, svcCtx, rds, refreshResp.User.ID, refreshResp.Token)
	_, loginJTI := tokenSessionClaimsForTest(loginResp.Token, svcCtx.CurrentConfig().JwtSecret)
	_, refreshJTI := tokenSessionClaimsForTest(refreshResp.Token, svcCtx.CurrentConfig().JwtSecret)
	if refreshSID != loginSID || refreshJTI == "" || refreshJTI == loginJTI {
		t.Fatalf("refresh claims sid=%q jti=%q, want sid=%q and a new jti", refreshSID, refreshJTI, loginSID)
	}
	requireSessionTokenNotCurrent(t, rds, refreshResp.User.ID, loginSID, loginResp.Token)
	requireSessionIndexMembers(t, rds, refreshResp.User.ID, []string{registerSID, refreshSID})

	logoutCtx := authFlowAuthenticatedContext(string(routealias.AuthLogout), http.MethodPost, "/api/auth/logout", "10.0.0.9", refreshResp)
	logoutResult := NewAuthLogic(logoutCtx, svcCtx).Logout()
	if logoutResult == nil || !logoutResult.IsSuccess() || logoutResult.Code != codes.Success {
		t.Fatalf("Logout() = %+v, want success", logoutResult)
	}
	requireSessionMissing(t, rds, refreshResp.User.ID, refreshSID)
	requireSessionIndexMembers(t, rds, refreshResp.User.ID, []string{registerSID})

	requireAuthFlowEvents(t, *seen, []authFlowEventWant{
		{action: AuthEventActionRegisterSuccess, reason: AuthEventReasonSessionCreated, route: string(routealias.AuthRegister)},
		{action: AuthEventActionLoginSuccess, reason: AuthEventReasonSessionCreated, route: string(routealias.AuthLogin)},
		{action: AuthEventActionRefreshSuccess, reason: AuthEventReasonSessionRotated, route: string(routealias.AuthRefresh)},
		{action: AuthEventActionLogoutSuccess, reason: AuthEventReasonCurrentSessionDeleted, route: string(routealias.AuthLogout)},
	})
}

// TestRegisterRollsBackDatabaseWhenSessionCreationFails 确保 Redis 会话失败不会留下无法重试的半注册账号。
func TestRegisterRollsBackDatabaseWhenSessionCreationFails(t *testing.T) {
	svcCtx, _, _ := newAuthFlowTestService(t)
	svcCtx.Rds = nil
	result := NewAuthLogic(authFlowContext(AuthEventActionRegisterSuccess, string(routealias.AuthRegister), http.MethodPost, "/api/auth/register", "10.0.0.8"), svcCtx).Register(&types.RegisterReq{
		Username: "rollback_user",
		Password: "P@ssw0rd!",
	})
	if result == nil || result.Code != codes.ServerError {
		t.Fatalf("Register() = %+v, want server error", result)
	}
	identity, err := model.FindUserIdentity(svcCtx.WriteDB(svc.DatabaseMain), model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, "rollback_user", svcCtx.CurrentConfig().AppKey)
	if err != nil {
		t.Fatalf("FindUserIdentity() error = %v", err)
	}
	if identity != nil {
		t.Fatalf("identity = %+v, want transaction rollback", identity)
	}
}

// TestLoginKeepsDatabaseStateWhenSessionCreationFails 确保 Redis 不可用时不会把未完成的登录写成最后登录记录。
func TestLoginKeepsDatabaseStateWhenSessionCreationFails(t *testing.T) {
	svcCtx, _, _ := newAuthFlowTestService(t)
	registerResp := requireAuthTokenResp(t, NewAuthLogic(authFlowContext(
		AuthEventActionRegisterSuccess,
		string(routealias.AuthRegister),
		http.MethodPost,
		"/api/auth/register",
		"10.0.0.8",
	), svcCtx).Register(&types.RegisterReq{
		Username: "session_failure_user",
		Password: "P@ssw0rd!",
	}), codes.CreateSuccess)
	cfg := svcCtx.CurrentConfig()
	before, err := model.FindUserByIdentity(
		svcCtx.WriteDB(svc.DatabaseMain),
		model.UserIdentityTypeUsername,
		model.UserIdentityProviderLocal,
		"session_failure_user",
		cfg.AppKey,
		cfg.User.RouteShardCount,
	)
	if err != nil || before == nil {
		t.Fatalf("FindUserByIdentity(before) user=%+v error=%v", before, err)
	}
	if before.ID != registerResp.User.ID {
		t.Fatalf("registered user id=%d, database user id=%d", registerResp.User.ID, before.ID)
	}

	svcCtx.Rds = nil
	result := NewAuthLogic(authFlowContext(
		AuthEventActionLoginSuccess,
		string(routealias.AuthLogin),
		http.MethodPost,
		"/api/auth/login",
		"10.0.0.9",
	), svcCtx).Login(&types.LoginReq{
		IdentityType:  types.LoginIdentityTypeUsername,
		IdentityValue: "session_failure_user",
		Password:      "P@ssw0rd!",
	})
	if result == nil || result.Code != codes.ServerError {
		t.Fatalf("Login() = %+v, want server error", result)
	}
	after, err := model.FindUserByIdentity(
		svcCtx.WriteDB(svc.DatabaseMain),
		model.UserIdentityTypeUsername,
		model.UserIdentityProviderLocal,
		"session_failure_user",
		cfg.AppKey,
		cfg.User.RouteShardCount,
	)
	if err != nil || after == nil {
		t.Fatalf("FindUserByIdentity(after) user=%+v error=%v", after, err)
	}
	if after.LastLoginIP != before.LastLoginIP || !after.LastLoginAt.Equal(before.LastLoginAt) {
		t.Fatalf("last login changed after session failure: before=(%s,%s) after=(%s,%s)",
			before.LastLoginIP, before.LastLoginAt, after.LastLoginIP, after.LastLoginAt)
	}
}

// TestLoginRemovesCreatedSessionWhenDatabaseUpdateFails 确保最后登录信息提交失败不会遗留未返回给客户端的新会话。
func TestLoginRemovesCreatedSessionWhenDatabaseUpdateFails(t *testing.T) {
	svcCtx, rds, _ := newAuthFlowTestService(t)
	registerResp := requireAuthTokenResp(t, NewAuthLogic(authFlowContext(
		AuthEventActionRegisterSuccess,
		string(routealias.AuthRegister),
		http.MethodPost,
		"/api/auth/register",
		"10.0.0.8",
	), svcCtx).Register(&types.RegisterReq{
		Username: "database_failure_user",
		Password: "P@ssw0rd!",
	}), codes.CreateSuccess)
	registerSID := requireSessionToken(t, svcCtx, rds, registerResp.User.ID, registerResp.Token)

	db := svcCtx.WriteDB(svc.DatabaseMain)
	const callbackName = "test:reject_login_last_login_update"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == model.TableNameUser {
			tx.AddError(fmt.Errorf("injected last login update failure"))
		}
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Update().Remove(callbackName)
	})

	result := NewAuthLogic(authFlowContext(
		AuthEventActionLoginSuccess,
		string(routealias.AuthLogin),
		http.MethodPost,
		"/api/auth/login",
		"10.0.0.9",
	), svcCtx).Login(&types.LoginReq{
		IdentityType:  types.LoginIdentityTypeUsername,
		IdentityValue: "database_failure_user",
		Password:      "P@ssw0rd!",
	})
	if result == nil || result.Code != codes.DBError {
		t.Fatalf("Login() = %+v, want database error", result)
	}
	requireSessionIndexMembers(t, rds, registerResp.User.ID, []string{registerSID})
}

// TestRuntimeSyncRequiresCommittedAuthVersion 确保内网会话失效只接受主库已提交的新认证版本。
func TestRuntimeSyncRequiresCommittedAuthVersion(t *testing.T) {
	svcCtx, rds, _ := newAuthFlowTestService(t)
	registerResp := requireAuthTokenResp(t, NewAuthLogic(authFlowContext(AuthEventActionRegisterSuccess, string(routealias.AuthRegister), http.MethodPost, "/api/auth/register", "10.0.0.8"), svcCtx).Register(&types.RegisterReq{
		Username: "version_user",
		Password: "P@ssw0rd!",
	}), codes.CreateSuccess)
	sessionID := requireSessionToken(t, svcCtx, rds, registerResp.User.ID, registerResp.Token)

	// 测试直接模拟后台敏感变更事务已提交；生产通用 UpdateUser 不允许修改认证版本。
	if err := svcCtx.WriteDB(svc.DatabaseMain).Table(model.TableNameUser).Where("id = ?", registerResp.User.ID).Update("auth_version", uint64(2)).Error; err != nil {
		t.Fatalf("commit auth_version error = %v", err)
	}
	stale := NewAuthLogic(context.Background(), svcCtx).SyncUserRuntime(&types.UserRuntimeSyncReq{
		ID: registerResp.User.ID, Sessions: true, AuthVersion: 3,
	})
	if stale == nil || stale.Code != codes.ServerError {
		t.Fatalf("SyncUserRuntime(stale) = %+v, want server error", stale)
	}
	requireSessionToken(t, svcCtx, rds, registerResp.User.ID, registerResp.Token)

	synced := NewAuthLogic(context.Background(), svcCtx).SyncUserRuntime(&types.UserRuntimeSyncReq{
		ID: registerResp.User.ID, Sessions: true, AuthVersion: 2,
	})
	if synced == nil || synced.Code != codes.UpdateSuccess {
		t.Fatalf("SyncUserRuntime(committed) = %+v, want update success", synced)
	}
	requireSessionMissing(t, rds, registerResp.User.ID, sessionID)
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
	if err := db.AutoMigrate(&authFlowUserSQLite{}); err != nil {
		t.Fatalf("AutoMigrate(User) error = %v", err)
	}
	if err := migrateAuthFlowUserIdentityTables(db); err != nil {
		t.Fatalf("migrateAuthFlowUserIdentityTables() error = %v", err)
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
	ID              int64     `gorm:"column:id;type:integer;primaryKey;autoIncrement:true;index:idx_user_shard_no_id,priority:2;index:idx_user_status_id,priority:2"` // 主键
	ShardNo         int       `gorm:"column:shard_no;type:int;not null;default:0;index:idx_user_shard_no_id,priority:1"`                                              // ID 哈希分片
	Username        string    `gorm:"column:username;type:varchar(32);not null;uniqueIndex:uk_user_username"`                                                         // 用户名
	Nickname        string    `gorm:"column:nickname;type:varchar(64);not null;default:''"`                                                                           // 用户昵称
	PasswordHash    string    `gorm:"column:password_hash;type:varchar(255);not null"`                                                                                // 密码哈希
	EmailCiphertext string    `gorm:"column:email_ciphertext;type:varchar(512);not null;default:''"`                                                                  // 邮箱密文
	EmailHash       string    `gorm:"column:email_hash;type:char(64);not null;default:'';index:idx_user_email_hash"`                                                  // 邮箱查询哈希
	EmailMasked     string    `gorm:"column:email_masked;type:varchar(128);not null;default:''"`                                                                      // 邮箱脱敏展示值
	EmailKeyVersion string    `gorm:"column:email_key_version;type:varchar(32);not null;default:''"`                                                                  // 邮箱密钥版本
	PhoneCiphertext string    `gorm:"column:phone_ciphertext;type:varchar(512);not null;default:''"`                                                                  // 手机号密文
	PhoneHash       string    `gorm:"column:phone_hash;type:char(64);not null;default:'';index:idx_user_phone_hash"`                                                  // 手机号查询哈希
	PhoneMasked     string    `gorm:"column:phone_masked;type:varchar(32);not null;default:''"`                                                                       // 手机号脱敏展示值
	PhoneKeyVersion string    `gorm:"column:phone_key_version;type:varchar(32);not null;default:''"`                                                                  // 手机号密钥版本
	Avatar          string    `gorm:"column:avatar;type:varchar(255);not null;default:''"`                                                                            // 头像
	Status          int       `gorm:"column:status;type:tinyint;not null;default:1;index:idx_user_status_id,priority:1"`                                              // 用户状态
	AuthVersion     uint64    `gorm:"column:auth_version;type:bigint unsigned;not null;default:1"`                                                                    // 认证版本
	LastLoginAt     time.Time `gorm:"column:last_login_at;type:datetime"`                                                                                             // 最后登录时间
	LastLoginIP     string    `gorm:"column:last_login_ip;type:varchar(45);not null;default:''"`                                                                      // 最后登录 IP
	CreatedAt       time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                             // 创建时间
	UpdatedAt       time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"`                                                             // 更新时间
}

// TableName 返回认证流程 SQLite 用户测试模型映射的真实表名。
func (*authFlowUserSQLite) TableName() string {
	return model.TableNameUser
}

// authFlowUserIdentitySQLite 使用 SQLite 创建用户身份索引表，业务读写仍走 model.UserIdentity。
type authFlowUserIdentitySQLite struct {
	ID            uint64    `gorm:"column:id;type:integer;primaryKey;autoIncrement:true"`               // 主键
	Provider      string    `gorm:"column:provider;type:varchar(32);not null;default:''"`               // 三方提供方
	IdentityValue string    `gorm:"column:identity_value;type:varchar(191);not null;default:''"`        // 身份值
	IdentityHash  string    `gorm:"column:identity_hash;type:char(64);not null;default:''"`             // 联系方式身份哈希
	UserID        int64     `gorm:"column:user_id;type:integer;not null"`                               // 用户 ID
	UserShardNo   int       `gorm:"column:user_shard_no;type:int;not null"`                             // 用户分片
	CreatedAt     time.Time `gorm:"column:created_at;type:datetime;not null;default:CURRENT_TIMESTAMP"` // 创建时间
	UpdatedAt     time.Time `gorm:"column:updated_at;type:datetime;not null;default:CURRENT_TIMESTAMP"` // 更新时间
}

// TableName 返回认证流程 SQLite 身份索引测试模型映射的真实表名。
func (*authFlowUserIdentitySQLite) TableName() string {
	return model.TableNameUserIdentityUsername
}

// migrateAuthFlowUserIdentityTables 创建认证流程需要的四张身份索引表。
func migrateAuthFlowUserIdentityTables(db *gorm.DB) error {
	for _, tableName := range []string{
		model.TableNameUserIdentityUsername,
		model.TableNameUserIdentityEmail,
		model.TableNameUserIdentityPhone,
		model.TableNameUserIdentityOAuth,
	} {
		if err := db.Table(tableName).AutoMigrate(&authFlowUserIdentitySQLite{}); err != nil {
			return err
		}
		if err := createAuthFlowUserIdentitySQLiteIndexes(db, tableName); err != nil {
			return err
		}
	}
	return nil
}

// createAuthFlowUserIdentitySQLiteIndexes 使用表名前缀规避 SQLite 全库索引名唯一限制。
func createAuthFlowUserIdentitySQLiteIndexes(db *gorm.DB, tableName string) error {
	var statements []string
	switch tableName {
	case model.TableNameUserIdentityUsername:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_identity_value` ON `%s` (`identity_value`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user` ON `%s` (`user_id`)", tableName, tableName),
		}
	case model.TableNameUserIdentityEmail, model.TableNameUserIdentityPhone:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_identity_hash` ON `%s` (`identity_hash`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user` ON `%s` (`user_id`)", tableName, tableName),
		}
	case model.TableNameUserIdentityOAuth:
		statements = []string{
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_provider_value` ON `%s` (`provider`, `identity_value`)", tableName, tableName),
			fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS `%s_user_provider` ON `%s` (`user_id`, `provider`)", tableName, tableName),
		}
	default:
		return fmt.Errorf("unknown identity table %s", tableName)
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
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
		sessionID, _ := tokenSessionClaimsForTest(tokenResp.Token, "test-secret-please-change")
		requestctx.SetSessionID(ctx, sessionID)
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

// requireSessionToken 断言 Redis session token 存在并返回 token 中的稳定 sid。
func requireSessionToken(t *testing.T, svcCtx *svc.ServiceContext, rds redis.UniversalClient, userID int64, token string) string {
	t.Helper()
	sessionID, jti := tokenSessionClaimsForTest(token, svcCtx.CurrentConfig().JwtSecret)
	if sessionID == "" || jti == "" {
		t.Fatal("token sid or jti is empty")
	}
	got, err := rds.HGet(context.Background(), keys.UserSessionHashKey(userID), sessionID).Result()
	if err != nil {
		t.Fatalf("Get(session %s) error = %v", sessionID, err)
	}
	if got != token {
		t.Fatalf("session token mismatch sid=%s", sessionID)
	}
	return sessionID
}

// requireSessionMissing 断言指定 sid 对应的 Redis session 已不存在。
func requireSessionMissing(t *testing.T, rds redis.UniversalClient, userID int64, sessionID string) {
	t.Helper()
	exists, err := rds.HExists(context.Background(), keys.UserSessionHashKey(userID), sessionID).Result()
	if err != nil {
		t.Fatalf("HExists(session %s) error = %v", sessionID, err)
	}
	if exists {
		t.Fatalf("session %s exists, want missing", sessionID)
	}
}

// requireSessionTokenNotCurrent 断言指定旧 token 已被同 sid 下的新 token 覆盖。
func requireSessionTokenNotCurrent(t *testing.T, rds redis.UniversalClient, userID int64, sessionID string, oldToken string) {
	t.Helper()
	current, err := rds.HGet(context.Background(), keys.UserSessionHashKey(userID), sessionID).Result()
	if err != nil {
		t.Fatalf("HGet(session %s) error = %v", sessionID, err)
	}
	if current == oldToken {
		t.Fatalf("session %s still stores the old token", sessionID)
	}
}

// requireSessionIndexMembers 断言用户 session 索引中仅包含期望的 sid 集合。
func requireSessionIndexMembers(t *testing.T, rds redis.UniversalClient, userID int64, want []string) {
	t.Helper()
	got, err := rds.ZRange(context.Background(), keys.UserSessionIndexKey(userID), 0, -1).Result()
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
