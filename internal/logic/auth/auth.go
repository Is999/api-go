package auth

import (
	"context"
	"fmt"
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
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// 认证入口限流动作名称。
const (
	authRateLimitActionLoginIP       = "login_ip"       // 登录 IP 维度
	authRateLimitActionLoginIdentity = "login_identity" // 登录身份维度
	authRateLimitActionRegisterIP    = "register_ip"    // 注册 IP 维度
)

// 前台用户 ID 生成命名空间。
const (
	userIDNamespace = "user" // api/admin 写同一用户表必须使用同一业务命名空间
)

// 批量会话操作保护边界。
const (
	maxUserSessionInvalidateBatch = 100 // 批量失效用户会话时单批删除的 session key 数
)

// ErrAuthRateLimited 表示认证入口触发限流。
var ErrAuthRateLimited = errors.New("认证入口触发限流")

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
		LastLoginAt:  now,
		LastLoginIP:  l.ClientIP(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if user.Nickname == "" {
		user.Nickname = user.Username
	}
	if err = model.CreateUserWithIdentities(l.Svc.WriteDB(svc.DatabaseMain), user, cfg.User.RouteShardCount, cfg.AppKey); err != nil {
		if corelogic.IsMySQLDuplicateEntryError(err) {
			return types.NewBizResult(codes.UserAlreadyExists).
				SetI18nMessage(i18n.MsgKeyUserAlreadyExists).
				WithError(errors.Errorf("AuthLogic.Register 登录身份[%s]已存在", req.Username))
		}
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Register 创建用户[%s]", req.Username).ToBizResult()
	}
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 创建用户[%s]会话失败", req.Username).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionRegisterSuccess,
		UserID:   user.ID,
		Identity: model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, user.Username),
		JTI:      created.JTI,
		Reason:   AuthEventReasonSessionCreated,
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
	user, err := model.FindUserByIdentity(l.Svc.WriteDB(svc.DatabaseMain), req.IdentityType, model.UserIdentityProviderLocal, req.IdentityValue, cfg.AppKey)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 查询登录身份类型[%s]", req.IdentityType).ToBizResult()
	}
	if user == nil {
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
	}); err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 更新用户[%s]登录信息", user.Username).ToBizResult()
	}
	user.LastLoginAt = now
	user.LastLoginIP = l.ClientIP()
	user.UpdatedAt = now
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Login 创建用户[%s]会话失败", user.Username).ToBizResult()
	}
	l.clearAuthRateLimit(authRateLimitActionLoginIP, l.ClientIP())
	l.clearAuthRateLimit(authRateLimitActionLoginIdentity, identitySubject)
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionLoginSuccess,
		UserID:   user.ID,
		Identity: identitySubject,
		JTI:      created.JTI,
		Reason:   AuthEventReasonSessionCreated,
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
	previousJTI := ""
	if meta := l.Meta(); meta != nil {
		previousJTI = tokenJTI(meta.AccessToken, l.Svc.CurrentConfig().JwtSecret)
	}
	resp, err := l.rotateSession(user, previousJTI)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Refresh 用户 ID[%d]轮换会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionRefreshSuccess,
		UserID:   user.ID,
		Identity: model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, user.Username),
		JTI:      tokenJTI(resp.Token, l.Svc.CurrentConfig().JwtSecret),
		Reason:   AuthEventReasonSessionRotated,
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
	jti := tokenJTI(l.AccessToken(), l.Svc.CurrentConfig().JwtSecret)
	if jti == "" {
		return types.NewBizResult(codes.TokenInvalid).
			SetI18nMessage(i18n.MsgKeyTokenInvalid).
			WithError(errors.New("AuthLogic.Logout 当前 token 缺少 jti"))
	}
	if err := l.deleteUserSession(ctxUser.ID, jti); err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Logout 用户 ID[%d]清理会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionLogoutSuccess,
		UserID:   ctxUser.ID,
		Identity: model.UserIdentitySubject(model.UserIdentityTypeUsername, model.UserIdentityProviderLocal, ctxUser.Name),
		JTI:      jti,
		Reason:   AuthEventReasonCurrentSessionDeleted,
	})
	return types.NewBizResult(codes.Success).
		SetI18nMessage(i18n.MsgKeyLogoutSuccess)
}

// createdSession 表示已写入 Redis 的新会话。
type createdSession struct {
	Response *types.AuthTokenResp // Response 表示返回给客户端的 token 数据
	JTI      string               // JTI 表示本次新会话的 JWT ID
}

// createSessionWithJTI 生成 JWT、写入 Redis 会话并返回内部 jti。
func (l *AuthLogic) createSessionWithJTI(user *model.User) (*createdSession, error) {
	if user == nil {
		return nil, errors.New("用户为空")
	}
	jti := strings.ReplaceAll(uuid.NewString(), "-", "")
	token, expiresAt, err := l.generateJWT(user, jti)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if l.Redis() == nil {
		return nil, errors.New("Redis 未初始化")
	}
	ttlSeconds := l.sessionTTL()
	sessionExpiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
	if err = l.Redis().Set(l.Ctx, l.userSessionKey(user.ID, jti), token, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
		return nil, errors.Wrapf(err, "写入用户会话失败 user_id=%d jti=%s", user.ID, jti)
	}
	if err = l.addUserSessionIndex(user.ID, jti, sessionExpiresAt, ttlSeconds); err != nil {
		_ = l.deleteUserSession(user.ID, jti)
		return nil, errors.Wrapf(err, "写入用户会话索引失败 user_id=%d jti=%s", user.ID, jti)
	}
	profile := userlogic.BuildUserProfile(user)
	userLogic := userlogic.NewUserLogic(l.Ctx, l.Svc)
	// 用户资料缓存只做加速，写入失败不影响已创建的 Redis 会话。
	_ = userLogic.CacheUserProfile(user.ID, profile)
	return &createdSession{
		JTI: jti,
		Response: &types.AuthTokenResp{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      profile,
		},
	}, nil
}

// rotateSession 创建新会话后删除原会话，删除失败时回滚新会话。
func (l *AuthLogic) rotateSession(user *model.User, previousJTI string) (*types.AuthTokenResp, error) {
	previousJTI = strings.TrimSpace(previousJTI)
	if user == nil {
		return nil, errors.New("用户为空")
	}
	if previousJTI == "" {
		return nil, errors.New("原会话 jti 为空")
	}
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if err := l.deleteUserSession(user.ID, previousJTI); err != nil {
		_ = l.deleteUserSession(user.ID, created.JTI)
		return nil, errors.Wrapf(err, "删除原用户会话失败 user_id=%d previous_jti=%s", user.ID, previousJTI)
	}
	return created.Response, nil
}

// generateJWT 生成包含用户、站点和 jti 信息的访问令牌。
func (l *AuthLogic) generateJWT(user *model.User, jti string) (string, int64, error) {
	cfg := l.Svc.CurrentConfig()
	now := time.Now()
	expiresAt := now.Add(time.Duration(cfg.JwtExpiresIn) * time.Second).Unix()
	claims := jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"jti":      jti,
		"iss":      cfg.Auth.Issuer,
		"app_id":   strings.TrimSpace(cfg.AppID),
		"iat":      now.Unix(),
		"exp":      expiresAt,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JwtSecret))
	return tokenString, expiresAt, errors.Tag(err)
}

// userSessionKey 生成当前站点下的用户会话 Redis Key。
func (l *AuthLogic) userSessionKey(userID int64, jti string) string {
	return l.AppRedisKey(fmt.Sprintf(keys.UserSession, userID, strings.TrimSpace(jti)))
}

// userSessionIndexKey 生成当前站点下的用户会话 jti 索引 Redis Key。
func (l *AuthLogic) userSessionIndexKey(userID int64) string {
	return l.AppRedisKey(fmt.Sprintf(keys.UserSessionIndex, userID))
}

// deleteUserSession 删除指定 jti 对应的用户会话。
func (l *AuthLogic) deleteUserSession(userID int64, jti string) error {
	jti = strings.TrimSpace(jti)
	if userID <= 0 || jti == "" {
		return errors.New("用户会话标识不能为空")
	}
	if err := l.RdsDelKeys(l.userSessionKey(userID, jti)); err != nil {
		return errors.Tag(err)
	}
	return errors.Tag(l.removeUserSessionIndex(userID, jti))
}

// InvalidateUserSessions 按用户会话索引批量删除全部登录态。
func (l *AuthLogic) InvalidateUserSessions(userID int64) error {
	if userID <= 0 {
		return errors.New("用户 ID 不能为空")
	}
	if l.Redis() == nil {
		return errors.New("Redis 未初始化")
	}
	indexKey := l.userSessionIndexKey(userID)
	if err := l.pruneExpiredUserSessionIndex(userID); err != nil {
		return errors.Tag(err)
	}
	jtis, err := l.Redis().ZRange(l.Ctx, indexKey, 0, -1).Result()
	if err != nil {
		return errors.Wrapf(err, "读取用户会话索引失败 user_id=%d", userID)
	}
	sessionKeys := make([]string, 0, len(jtis))
	seen := make(map[string]struct{}, len(jtis))
	for _, jti := range jtis {
		jti = strings.TrimSpace(jti)
		if jti == "" {
			continue
		}
		if _, ok := seen[jti]; ok {
			continue
		}
		seen[jti] = struct{}{}
		sessionKeys = append(sessionKeys, l.userSessionKey(userID, jti))
	}
	invalidatedCount := len(sessionKeys)
	for len(sessionKeys) > 0 {
		batchSize := maxUserSessionInvalidateBatch
		if len(sessionKeys) < batchSize {
			batchSize = len(sessionKeys)
		}
		if err := l.RdsDelKeys(sessionKeys[:batchSize]...); err != nil {
			return errors.Wrapf(err, "批量删除用户会话失败 user_id=%d", userID)
		}
		sessionKeys = sessionKeys[batchSize:]
	}
	if err := l.RdsDelKeys(indexKey); err != nil {
		return errors.Tag(err)
	}
	l.emitAuthEvent(AuthEventInput{
		Action: AuthEventActionSessionInvalidateAll,
		UserID: userID,
		Reason: AuthEventReasonUserSessionsInvalidated,
		Count:  invalidatedCount,
	})
	return nil
}

// addUserSessionIndex 写入用户 jti 索引，并顺带清理已过期的索引成员。
func (l *AuthLogic) addUserSessionIndex(userID int64, jti string, expiresAt int64, ttlSeconds int64) error {
	jti = strings.TrimSpace(jti)
	if userID <= 0 || jti == "" {
		return errors.New("用户会话索引标识不能为空")
	}
	if l.Redis() == nil {
		return errors.New("Redis 未初始化")
	}
	if err := l.pruneExpiredUserSessionIndex(userID); err != nil {
		return errors.Tag(err)
	}
	indexKey := l.userSessionIndexKey(userID)
	if err := l.Redis().ZAdd(l.Ctx, indexKey, redis.Z{
		Score:  float64(expiresAt),
		Member: jti,
	}).Err(); err != nil {
		return errors.Wrapf(err, "写入用户会话索引失败 user_id=%d jti=%s", userID, jti)
	}
	if ttlSeconds <= 0 {
		ttlSeconds = l.sessionTTL()
	}
	if err := l.Redis().Expire(l.Ctx, indexKey, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
		_ = l.removeUserSessionIndex(userID, jti)
		return errors.Wrapf(err, "设置用户会话索引过期时间失败 user_id=%d", userID)
	}
	return nil
}

// removeUserSessionIndex 删除用户 jti 索引成员。
func (l *AuthLogic) removeUserSessionIndex(userID int64, jti string) error {
	jti = strings.TrimSpace(jti)
	if userID <= 0 || jti == "" || l.Redis() == nil {
		return nil
	}
	return errors.Tag(l.Redis().ZRem(l.Ctx, l.userSessionIndexKey(userID), jti).Err())
}

// pruneExpiredUserSessionIndex 删除已自然过期的 jti 索引成员。
func (l *AuthLogic) pruneExpiredUserSessionIndex(userID int64) error {
	if userID <= 0 || l.Redis() == nil {
		return nil
	}
	now := time.Now().Unix()
	return errors.Tag(l.Redis().ZRemRangeByScore(l.Ctx, l.userSessionIndexKey(userID), "-inf", fmt.Sprintf("%d", now)).Err())
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
	if err := l.Redis().Get(l.Ctx, lockKey).Err(); err == nil {
		return ErrAuthRateLimited
	} else if err != nil && !errors.Is(err, redis.Nil) {
		return errors.Wrapf(err, "读取认证限流锁失败 action=%s", action)
	}
	count, err := l.Redis().Incr(l.Ctx, countKey).Result()
	if err != nil {
		return errors.Wrapf(err, "写入认证限流计数失败 action=%s", action)
	}
	if count == 1 {
		if err := l.Redis().Expire(l.Ctx, countKey, time.Duration(cfg.WindowSeconds)*time.Second).Err(); err != nil {
			return errors.Wrapf(err, "设置认证限流窗口失败 action=%s", action)
		}
	}
	if count > int64(cfg.MaxAttempts) {
		if err := l.Redis().Set(l.Ctx, lockKey, "1", time.Duration(cfg.LockSeconds)*time.Second).Err(); err != nil {
			return errors.Wrapf(err, "写入认证限流锁失败 action=%s", action)
		}
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
	_ = l.RdsDelKeys(countKey, lockKey)
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

// tokenJTI 从访问令牌中解析 jti，解析失败时返回空字符串。
func tokenJTI(tokenString string, secret string) string {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" || strings.TrimSpace(secret) == "" {
		return ""
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || token == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(claims["jti"]))
}
