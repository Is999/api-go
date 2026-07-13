package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	codes "api/common/codes"
	i18n "api/common/i18n"
	"api/common/idgen"
	keys "api/common/rediskeys"
	"api/internal/config"
	corelogic "api/internal/logic"
	userlogic "api/internal/logic/user"
	"api/internal/model"
	"api/internal/svc"
	"api/internal/types"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 认证入口限流动作名称。
const (
	authRateLimitActionLoginIP       = "login_ip"       // 登录 IP 维度
	authRateLimitActionLoginIdentity = "login_identity" // 登录身份维度
	authRateLimitActionRegisterIP    = "register_ip"    // 注册 IP 维度
)

const (
	// authLoginDummyPasswordHash 用于不存在账号的等时 bcrypt 校验，避免通过响应耗时枚举用户账号。
	authLoginDummyPasswordHash = "$2y$10$ory3FZfUy1VExaUHmEkeluYtVtP/4CiCCfeSPfD12T9dbpWqO52Eq"
)

// 前台用户 ID 生成命名空间。
const (
	userIDNamespace = "user" // api/admin 写同一用户表必须使用同一业务命名空间
)

// 前台用户会话保护边界。
const (
	maxUserSessions            = 8               // 单个用户最多保留的有效会话数，超出时原子淘汰最早过期会话
	registrationCleanupTimeout = 3 * time.Second // 注册事务失败后的 Redis 补偿超时
)

// 认证与会话内部错误哨兵。
var (
	// ErrAuthRateLimited 表示认证入口触发限流。
	ErrAuthRateLimited = errors.New("认证入口触发限流")
	// ErrSessionStale 表示刷新使用的旧会话已被消费或失效。
	ErrSessionStale = errors.New("用户会话已失效")
	// ErrAuthVersionMismatch 表示 Redis 认证版本已领先于调用方数据库快照。
	ErrAuthVersionMismatch = errors.New("用户认证版本不一致")
)

// AuthLogic 承载前台注册、登录和会话刷新逻辑。
type AuthLogic struct {
	*corelogic.BaseLogic // 复用上下文、日志、数据库和缓存等公共能力
}

// NewAuthLogic 创建前台认证逻辑对象。
func NewAuthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthLogic {
	return &AuthLogic{BaseLogic: corelogic.NewBaseLogicWithContext(ctx, svcCtx)}
}

// Register 注册前台用户并创建登录态。
func (l *AuthLogic) Register(req *types.RegisterReq) *types.BizResult {
	if err := req.Validate(); err != nil {
		return types.ParamErrorResult(err).
			WithError(err)
	}
	cfg := l.Svc.CurrentConfig()
	if !cfg.Auth.RegisterEnabled {
		return types.NewBizResult(codes.RegisterDisabled).
			SetI18nMessage(i18n.MsgKeyRegisterDisabled).
			WithError(errors.New("AuthLogic.Register 注册入口未开放"))
	}
	if len(req.Password) < l.passwordMinLength() {
		err := errors.Errorf("密码长度不能少于 %d 位", l.passwordMinLength())
		return types.ParamErrorResult(err).
			WithError(err)
	}
	if err := l.checkAuthRateLimit(authRateLimitActionRegisterIP, l.ClientIP(), cfg.Auth.RegisterRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Identity: model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, req.Username),
				Reason:   AuthEventReasonRegisterIPRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	exists, err := model.FindUserIdentity(l.Svc.WriteDB(svc.DatabaseMain), model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, req.Username, cfg.AppKey)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Register 查询登录身份[%s]", req.Username).ToBizResult()
	}
	if exists != nil {
		return types.NewBizResult(codes.UserAlreadyExists).
			SetI18nMessage(i18n.MsgKeyUserAlreadyExists).
			WithError(errors.Errorf("AuthLogic.Register 登录身份[%s]已存在", req.Username))
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 生成密码哈希失败").ToBizResult()
	}
	userID, err := idgen.NextID(userIDNamespace)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 生成用户 ID失败").ToBizResult()
	}
	now := time.Now()
	user := &model.User{
		ID:           userID,
		ShardNo:      idgen.ShardNo(userID),
		Username:     req.Username,
		Nickname:     req.Nickname,
		PasswordHash: string(passwordHash),
		Email:        req.Email,
		Phone:        req.Phone,
		Status:       model.UserStatusEnabled,
		AuthVersion:  1,
		LastLoginAt:  now,
		LastLoginIP:  l.ClientIP(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if user.Nickname == "" {
		user.Nickname = user.Username
	}
	var created *createdSession
	var sessionErr error
	var sessionAttempted bool
	db := l.Svc.WriteDB(svc.DatabaseMain)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := model.CreateUserWithIdentitiesTx(tx, user, cfg.User.RouteShardCount, cfg.AppKey); err != nil {
			return errors.Tag(err)
		}
		sessionAttempted = true
		created, sessionErr = l.createSession(user)
		return errors.Tag(sessionErr)
	})
	if err != nil {
		if sessionAttempted {
			if cleanupErr := l.discardRegistrationRuntimeState(user.ID); cleanupErr != nil {
				err = errors.Wrapf(err, "注册事务失败且清理预创建会话失败 cleanup_error=%v", cleanupErr)
			}
		}
		if sessionErr != nil {
			return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 创建用户[%s]会话失败", req.Username).ToBizResult()
		}
		if corelogic.IsMySQLDuplicateEntryError(err) {
			return types.NewBizResult(codes.UserAlreadyExists).
				SetI18nMessage(i18n.MsgKeyUserAlreadyExists).
				WithError(errors.Errorf("AuthLogic.Register 登录身份[%s]已存在", req.Username))
		}
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Register 创建用户[%s]", req.Username).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:    AuthEventActionRegisterSuccess,
		UserID:    user.ID,
		Identity:  model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, user.Username),
		SessionID: created.SessionID,
		Reason:    AuthEventReasonSessionCreated,
	})
	return types.NewBizResult(codes.CreateSuccess).
		SetI18nMessage(i18n.MsgKeyCreateSuccess).
		WithData(created.Response)
}

// Login 校验账号密码并创建登录态。
func (l *AuthLogic) Login(req *types.LoginReq) *types.BizResult {
	if err := req.Validate(); err != nil {
		return types.ParamErrorResult(err).
			WithError(err)
	}
	cfg := l.Svc.CurrentConfig()
	if err := l.checkAuthRateLimit(authRateLimitActionLoginIP, l.ClientIP(), cfg.Auth.LoginRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Identity: loginIdentitySubject(req),
				Reason:   AuthEventReasonLoginIPRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	identitySubject := loginIdentitySubject(req)
	if err := l.checkAuthRateLimit(authRateLimitActionLoginIdentity, identitySubject, cfg.Auth.LoginRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Identity: identitySubject,
				Reason:   AuthEventReasonLoginIdentityRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	user, err := model.FindUserByIdentity(l.Svc.WriteDB(svc.DatabaseMain), req.IdentityType, model.UserIdentityProviderLocal, req.IdentityValue, cfg.AppKey, cfg.User.RouteShardCount)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 查询登录身份类型[%s]", req.IdentityType).ToBizResult()
	}
	if user == nil {
		// 用户不存在时仍执行固定哈希校验，使失败路径耗时接近，避免通过响应时间枚举账号。
		_ = bcrypt.CompareHashAndPassword([]byte(authLoginDummyPasswordHash), []byte(req.Password))
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			Identity: identitySubject,
			Reason:   AuthEventReasonInvalidPassword,
		})
		return invalidPasswordResult(errors.Errorf("AuthLogic.Login 登录身份类型[%s]不存在", req.IdentityType))
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			UserID:   user.ID,
			Identity: identitySubject,
			Reason:   AuthEventReasonInvalidPassword,
		})
		return invalidPasswordResult(errors.Errorf("AuthLogic.Login 登录身份类型[%s]密码错误", req.IdentityType))
	}
	if user.Status != model.UserStatusEnabled {
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			UserID:   user.ID,
			Identity: identitySubject,
			Reason:   AuthEventReasonUserDisabled,
		})
		return types.NewBizResult(codes.UserDisabled).
			SetI18nMessage(i18n.MsgKeyUserDisabled).
			WithError(errors.Errorf("AuthLogic.Login 用户[%s]已禁用", user.Username))
	}
	now := time.Now()
	if err = model.UpdateUser(l.Svc.WriteDB(svc.DatabaseMain), user.ID, map[string]any{
		"last_login_at": now,
		"last_login_ip": l.ClientIP(),
		"updated_at":    now,
	}, cfg.User.RouteShardCount); err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 更新用户[%s]登录信息", user.Username).ToBizResult()
	}
	user.LastLoginAt = now
	user.LastLoginIP = l.ClientIP()
	user.UpdatedAt = now
	created, err := l.createSession(user)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Login 创建用户[%s]会话失败", user.Username).ToBizResult()
	}
	l.clearAuthRateLimit(authRateLimitActionLoginIP, l.ClientIP())
	l.clearAuthRateLimit(authRateLimitActionLoginIdentity, identitySubject)
	l.emitAuthEvent(AuthEventInput{
		Action:    AuthEventActionLoginSuccess,
		UserID:    user.ID,
		Identity:  identitySubject,
		SessionID: created.SessionID,
		Reason:    AuthEventReasonSessionCreated,
	})
	return types.NewBizResult(codes.Success).
		SetI18nMessage(i18n.MsgKeySuccess).
		WithData(created.Response)
}

// Refresh 刷新当前用户访问令牌。
func (l *AuthLogic) Refresh() *types.BizResult {
	ctxUser := l.GetCtxUser()
	if ctxUser == nil || ctxUser.ID <= 0 {
		return types.NewBizResult(codes.Unauthorized).
			SetI18nMessage(i18n.MsgKeyUnauthorizedText).
			WithError(errors.New("AuthLogic.Refresh 当前请求未登录"))
	}
	user, err := userlogic.NewUserLogic(l.Ctx, l.Svc).GetActiveUserForAuth(ctxUser.ID)
	if err != nil {
		if errors.Is(err, userlogic.ErrUserNotFound) {
			return types.NewBizResult(codes.TokenInvalid).
				SetI18nMessage(i18n.MsgKeyTokenInvalid).
				WithError(corelogic.WrapLogicError(err, "AuthLogic.Refresh 用户 ID[%d]不存在", ctxUser.ID))
		}
		return types.NewBizResult(codes.UserDisabled).
			SetI18nMessage(i18n.MsgKeyUserDisabled).
			WithError(corelogic.WrapLogicError(err, "AuthLogic.Refresh 用户 ID[%d]状态无效", ctxUser.ID))
	}
	sessionID := ""
	if meta := l.Meta(); meta != nil {
		sessionID = meta.SessionID
	}
	if sessionID == "" {
		return types.NewBizResult(codes.TokenInvalid).
			SetI18nMessage(i18n.MsgKeyTokenInvalid).
			WithError(errors.New("AuthLogic.Refresh 当前 token 缺少 sid"))
	}
	resp, err := l.rotateSession(user, sessionID, l.AccessToken())
	if err != nil {
		if errors.Is(err, ErrSessionStale) || errors.Is(err, ErrAuthVersionMismatch) {
			return types.NewBizResult(codes.SessionExpired).
				SetI18nMessage(i18n.MsgKeySessionExpired).
				WithError(corelogic.WrapLogicError(err, "AuthLogic.Refresh 用户 ID[%d]原会话已失效", ctxUser.ID))
		}
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Refresh 用户 ID[%d]轮换会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:    AuthEventActionRefreshSuccess,
		UserID:    user.ID,
		Identity:  model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, user.Username),
		SessionID: sessionID,
		Reason:    AuthEventReasonSessionRotated,
	})
	return types.NewBizResult(codes.Success).
		SetI18nMessage(i18n.MsgKeySuccess).
		WithData(resp)
}

// Logout 清理当前用户登录态。
func (l *AuthLogic) Logout() *types.BizResult {
	ctxUser := l.GetCtxUser()
	if ctxUser == nil || ctxUser.ID <= 0 {
		return types.NewBizResult(codes.Unauthorized).
			SetI18nMessage(i18n.MsgKeyUnauthorizedText).
			WithError(errors.New("AuthLogic.Logout 当前请求未登录"))
	}
	sessionID := ""
	if meta := l.Meta(); meta != nil {
		sessionID = meta.SessionID
	}
	if sessionID == "" {
		return types.NewBizResult(codes.TokenInvalid).
			SetI18nMessage(i18n.MsgKeyTokenInvalid).
			WithError(errors.New("AuthLogic.Logout 当前 token 缺少 sid"))
	}
	if err := l.deleteUserSession(ctxUser.ID, sessionID); err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Logout 用户 ID[%d]清理会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:    AuthEventActionLogoutSuccess,
		UserID:    ctxUser.ID,
		Identity:  model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, ctxUser.Name),
		SessionID: sessionID,
		Reason:    AuthEventReasonCurrentSessionDeleted,
	})
	return types.NewBizResult(codes.Success).
		SetI18nMessage(i18n.MsgKeyLogoutSuccess)
}

// createdSession 表示已写入 Redis 的新会话。
type createdSession struct {
	Response  *types.AuthTokenResp // Response 表示返回给客户端的 token 数据
	SessionID string               // SessionID 表示一次登录会话内保持稳定的会话 ID
}

// createSession 生成独立 sid 与 jti，并原子写入 Redis 会话。
func (l *AuthLogic) createSession(user *model.User) (*createdSession, error) {
	if user == nil {
		return nil, errors.New("用户为空")
	}
	if user.AuthVersion == 0 {
		return nil, errors.New("用户认证版本不能为空")
	}
	sessionID := newTokenID()
	jti := newTokenID()
	token, expiresAt, err := l.generateJWT(user, sessionID, jti)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if l.Redis() == nil {
		return nil, errors.New("Redis 未初始化")
	}
	now := time.Now()
	ttlSeconds := l.sessionTTL()
	sessionExpiresAtMS := now.Add(time.Duration(ttlSeconds) * time.Second).UnixMilli()
	created, err := userSessionCreateScript.Run(
		l.Ctx,
		l.Redis(),
		keys.UserSessionKeys(user.ID),
		now.UnixMilli(),
		user.AuthVersion,
		sessionID,
		token,
		sessionExpiresAtMS,
		maxUserSessions,
	).Int64()
	if err != nil {
		return nil, errors.Wrapf(err, "原子创建用户会话失败 user_id=%d sid=%s", user.ID, sessionID)
	}
	if created < 0 {
		return nil, ErrAuthVersionMismatch
	}
	profile := userlogic.BuildUserProfile(user)
	userLogic := userlogic.NewUserLogic(l.Ctx, l.Svc)
	// 用户资料缓存只做加速，写入失败不影响已创建的 Redis 会话。
	_ = userLogic.CacheUserProfile(user.ID, profile)
	return &createdSession{
		SessionID: sessionID,
		Response: &types.AuthTokenResp{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      profile,
		},
	}, nil
}

// rotateSession 在稳定 sid 下原子比较完整旧 token 并写入新 token。
func (l *AuthLogic) rotateSession(user *model.User, sessionID string, previousToken string) (*types.AuthTokenResp, error) {
	sessionID = strings.TrimSpace(sessionID)
	previousToken = strings.TrimSpace(previousToken)
	if user == nil {
		return nil, errors.New("用户为空")
	}
	if user.AuthVersion == 0 {
		return nil, errors.New("用户认证版本不能为空")
	}
	if sessionID == "" || previousToken == "" {
		return nil, errors.New("原会话标识不能为空")
	}
	if l.Redis() == nil {
		return nil, errors.New("Redis 未初始化")
	}
	newJTI := newTokenID()
	newToken, expiresAt, err := l.generateJWT(user, sessionID, newJTI)
	if err != nil {
		return nil, errors.Tag(err)
	}
	now := time.Now()
	result, err := userSessionRotateScript.Run(
		l.Ctx,
		l.Redis(),
		keys.UserSessionKeys(user.ID),
		now.UnixMilli(),
		user.AuthVersion,
		sessionID,
		previousToken,
		newToken,
		now.Add(time.Duration(l.sessionTTL())*time.Second).UnixMilli(),
	).Int64()
	if err != nil {
		return nil, errors.Wrapf(err, "原子轮换用户会话失败 user_id=%d sid=%s", user.ID, sessionID)
	}
	switch result {
	case -1:
		return nil, ErrAuthVersionMismatch
	case 0:
		return nil, ErrSessionStale
	}
	profile := userlogic.BuildUserProfile(user)
	_ = userlogic.NewUserLogic(l.Ctx, l.Svc).CacheUserProfile(user.ID, profile)
	return &types.AuthTokenResp{Token: newToken, ExpiresAt: expiresAt, User: profile}, nil
}

// generateJWT 生成包含用户、站点、稳定 sid 和唯一 jti 的访问令牌。
func (l *AuthLogic) generateJWT(user *model.User, sessionID string, jti string) (string, int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	jti = strings.TrimSpace(jti)
	if user == nil || user.ID <= 0 || user.AuthVersion == 0 || sessionID == "" || jti == "" {
		return "", 0, errors.New("用户或认证版本非法")
	}
	cfg := l.Svc.CurrentConfig()
	now := time.Now()
	expiresAt := now.Add(time.Duration(cfg.JwtExpiresIn) * time.Second).Unix()
	claims := jwt.MapClaims{
		"sub":          strconv.FormatInt(user.ID, 10),
		"username":     user.Username,
		"sid":          sessionID,
		"jti":          jti,
		"iss":          cfg.Auth.Issuer,
		"app_id":       strings.TrimSpace(cfg.AppID),
		"auth_version": user.AuthVersion,
		"iat":          now.Unix(),
		"exp":          expiresAt,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JwtSecret))
	return tokenString, expiresAt, errors.Tag(err)
}

// deleteUserSession 删除指定 sid 对应的当前登录会话。
func (l *AuthLogic) deleteUserSession(userID int64, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if userID <= 0 || sessionID == "" {
		return errors.New("用户会话标识不能为空")
	}
	if l.Redis() == nil {
		return errors.New("Redis 未初始化")
	}
	_, err := userSessionDeleteScript.Run(
		l.Ctx,
		l.Redis(),
		[]string{keys.UserSessionHashKey(userID), keys.UserSessionIndexKey(userID)},
		sessionID,
	).Int64()
	return errors.Tag(err)
}

// discardRegistrationRuntimeState 使用独立超时上下文清理注册回滚后的会话和资料缓存。
func (l *AuthLogic) discardRegistrationRuntimeState(userID int64) error {
	if userID <= 0 || l.Redis() == nil {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), registrationCleanupTimeout)
	defer cancel()
	sessionErr := l.Redis().Del(cleanupCtx, keys.UserSessionKeys(userID)...).Err()
	profileErr := userlogic.NewUserLogic(cleanupCtx, l.Svc).DeleteUserProfileCache(userID)
	if sessionErr != nil && profileErr != nil {
		return errors.Wrapf(sessionErr, "清理用户资料缓存失败 profile_error=%v", profileErr)
	}
	if sessionErr != nil {
		return errors.Tag(sessionErr)
	}
	return errors.Tag(profileErr)
}

// InvalidateUserSessions 按已提交的数据库认证版本原子删除全部登录态。
func (l *AuthLogic) InvalidateUserSessions(userID int64, authVersion uint64) error {
	if userID <= 0 || authVersion == 0 {
		return errors.New("用户 ID 和认证版本不能为空")
	}
	if l.Redis() == nil {
		return errors.New("Redis 未初始化")
	}
	invalidatedCount, err := userSessionInvalidateScript.Run(
		l.Ctx,
		l.Redis(),
		keys.UserSessionKeys(userID),
		authVersion,
		l.authVersionFenceTTL(),
	).Int64()
	if err != nil {
		return errors.Wrapf(err, "原子失效用户会话失败 user_id=%d auth_version=%d", userID, authVersion)
	}
	if invalidatedCount < 0 {
		return ErrAuthVersionMismatch
	}
	l.emitAuthEvent(AuthEventInput{
		Action: AuthEventActionSessionInvalidateAll,
		UserID: userID,
		Reason: AuthEventReasonUserSessionsInvalidated,
		Count:  int(invalidatedCount),
	})
	return nil
}

// sessionTTL 返回 Redis 会话 TTL，不超过 JWT 过期时间。
func (l *AuthLogic) sessionTTL() int64 {
	cfg := l.Svc.CurrentConfig()
	jwtTTL := cfg.JwtExpiresIn
	if jwtTTL <= 0 {
		jwtTTL = 86400
	}
	if cfg.Auth.SessionTTLSeconds > 0 && cfg.Auth.SessionTTLSeconds < jwtTTL {
		return cfg.Auth.SessionTTLSeconds
	}
	return jwtTTL
}

// authVersionFenceTTL 返回认证版本栅栏 TTL，覆盖 JWT 最长存活期。
func (l *AuthLogic) authVersionFenceTTL() int64 {
	jwtTTL := l.Svc.CurrentConfig().JwtExpiresIn
	if jwtTTL <= 0 {
		return 86400
	}
	return jwtTTL
}

// checkAuthRateLimit 校验认证入口在 Redis 中的限流状态。
func (l *AuthLogic) checkAuthRateLimit(action, subject string, cfg config.AuthRateLimitConfig) error {
	cfg = normalizeAuthRateLimitConfig(action, cfg)
	if !cfg.Enabled {
		return nil
	}
	if l.Redis() == nil {
		return errors.Errorf("认证限流 Redis 未初始化 action=%s", action)
	}
	countKey, lockKey := l.authRateLimitKeys(action, subject)
	result, err := authRateLimitScript.Run(
		l.Ctx,
		l.Redis(),
		[]string{countKey, lockKey},
		cfg.WindowSeconds,
		cfg.MaxAttempts,
		cfg.LockSeconds,
	).Int64()
	if err != nil {
		return errors.Wrapf(err, "原子更新认证限流失败 action=%s", action)
	}
	if result < 0 {
		return ErrAuthRateLimited
	}
	return nil
}

// clearAuthRateLimit 在登录成功后清理当前主体的限流状态。
func (l *AuthLogic) clearAuthRateLimit(action, subject string) {
	if l == nil || l.Redis() == nil {
		return
	}
	countKey, lockKey := l.authRateLimitKeys(action, subject)
	_ = l.Redis().Del(l.Ctx, countKey, lockKey).Err()
}

// authRateLimitKeys 生成认证入口限流计数和锁定 Redis Key。
func (l *AuthLogic) authRateLimitKeys(action, subject string) (string, string) {
	action = strings.TrimSpace(action)
	if action == "" {
		action = "unknown"
	}
	subject = strings.ToLower(strings.TrimSpace(subject))
	if subject == "" {
		subject = "unknown"
	}
	subjectHash := utils.MD5(subject)
	return l.AppRedisKey(fmt.Sprintf(keys.AuthRateLimitCount, action, subjectHash)),
		l.AppRedisKey(fmt.Sprintf(keys.AuthRateLimitLock, action, subjectHash))
}

// normalizeAuthRateLimitConfig 补齐认证限流默认值。
func normalizeAuthRateLimitConfig(action string, cfg config.AuthRateLimitConfig) config.AuthRateLimitConfig {
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = 60
	}
	if cfg.LockSeconds <= 0 {
		cfg.LockSeconds = cfg.WindowSeconds
	}
	if cfg.MaxAttempts <= 0 {
		switch action {
		case authRateLimitActionRegisterIP:
			cfg.MaxAttempts = 3
		default:
			cfg.MaxAttempts = 5
		}
	}
	return cfg
}

// passwordMinLength 返回注册密码最小长度，未配置时使用 8 位。
func (l *AuthLogic) passwordMinLength() int {
	cfg := l.Svc.CurrentConfig()
	if cfg.Auth.PasswordMinLength > 0 {
		return cfg.Auth.PasswordMinLength
	}
	return 8
}

// loginIdentitySubject 返回密码登录限流和风控使用的身份主体。
func loginIdentitySubject(req *types.LoginReq) string {
	if req == nil {
		return ""
	}
	return model.UserIdentitySubject(req.IdentityType, model.UserIdentityProviderLocal, req.IdentityValue)
}

// invalidPasswordResult 返回统一账号或密码错误，避免暴露账号存在性。
func invalidPasswordResult(err error) *types.BizResult {
	return types.NewBizResult(codes.InvalidPassword).
		SetI18nMessage(i18n.MsgKeyInvalidPassword).
		WithError(err)
}

// authRateLimitResult 返回统一限流或内部错误响应。
func authRateLimitResult(err error) *types.BizResult {
	if errors.Is(err, ErrAuthRateLimited) {
		return types.NewBizResult(codes.RateLimit).
			SetI18nMessage(i18n.MsgKeyRateLimit).
			WithError(err)
	}
	return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.RateLimit").ToBizResult()
}

// newTokenID 生成不含分隔符的随机 token 标识。
func newTokenID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}
