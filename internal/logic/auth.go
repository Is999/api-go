package logic

import (
	"context"
	"fmt"
	"strings"
	"time"

	codes "gozero_api/common/codes"
	i18n "gozero_api/common/i18n"
	keys "gozero_api/common/rediskeys"
	"gozero_api/internal/config"
	"gozero_api/internal/model"
	"gozero_api/internal/svc"
	"gozero_api/internal/types"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// 前台账号输入长度边界。
const (
	usernameMinLength = 3  // 用户名最小长度
	usernameMaxLength = 32 // 用户名最大长度
)

// 认证入口限流动作名称。
const (
	authRateLimitActionLoginIP       = "login_ip"       // 登录 IP 维度
	authRateLimitActionLoginUsername = "login_username" // 登录用户名维度
	authRateLimitActionRegisterIP    = "register_ip"    // 注册 IP 维度
)

const (
	maxUserSessionInvalidateBatch = 100 // 批量失效用户会话时单批删除的 session key 数
)

// ErrAuthRateLimited 表示认证入口触发限流。
var ErrAuthRateLimited = errors.New("认证入口触发限流")

// AuthLogic 承载前台注册、登录和会话刷新逻辑。
type AuthLogic struct {
	*BaseLogic // 复用上下文、日志、数据库和缓存等公共能力
}

// NewAuthLogic 创建前台认证逻辑对象。
func NewAuthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthLogic {
	return &AuthLogic{BaseLogic: NewBaseLogicWithContext(ctx, svcCtx)}
}

// Register 注册前台用户并创建登录态。
func (l *AuthLogic) Register(req *types.RegisterReq) *types.BizResult {
	cfg := l.svc.CurrentConfig()
	if !cfg.Auth.RegisterEnabled {
		return types.NewBizResult(codes.RegisterDisabled).
			SetI18nMessage(i18n.MsgKeyRegisterDisabled).
			WithError(errors.New("AuthLogic.Register 注册入口未开放"))
	}
	if err := l.validateRegisterReq(req); err != nil {
		return types.NewBizResult(codes.ParamError).
			SetI18nMessage(i18n.MsgKeyParamErrorFormat, err.Error()).
			WithError(err)
	}
	if err := l.checkAuthRateLimit(authRateLimitActionRegisterIP, l.ClientIP(), cfg.Auth.RegisterRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Username: req.Username,
				Reason:   AuthEventReasonRegisterIPRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	exists, err := model.FindAPIUserByUsername(l.svc.WriteDB(svc.DatabaseMain), req.Username)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Register 查询用户名[%s]", req.Username).ToBizResult()
	}
	if exists != nil {
		return types.NewBizResult(codes.UserAlreadyExists).
			SetI18nMessage(i18n.MsgKeyUserAlreadyExists).
			WithError(errors.Errorf("AuthLogic.Register 用户名[%s]已存在", req.Username))
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 生成密码哈希失败").ToBizResult()
	}
	now := time.Now()
	user := &model.APIUser{
		Username:     strings.TrimSpace(req.Username),
		Nickname:     strings.TrimSpace(req.Nickname),
		PasswordHash: string(passwordHash),
		Email:        strings.TrimSpace(req.Email),
		Phone:        strings.TrimSpace(req.Phone),
		Status:       model.APIUserStatusEnabled,
		LastLoginAt:  now,
		LastLoginIP:  l.ClientIP(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if user.Nickname == "" {
		user.Nickname = user.Username
	}
	if err = model.CreateAPIUser(l.svc.WriteDB(svc.DatabaseMain), user); err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Register 创建用户[%s]", req.Username).ToBizResult()
	}
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Register 创建用户[%s]会话失败", req.Username).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionRegisterSuccess,
		UserID:   user.ID,
		Username: user.Username,
		JTI:      created.JTI,
		Reason:   AuthEventReasonSessionCreated,
	})
	return types.NewBizResult(codes.CreateSuccess).
		SetI18nMessage(i18n.MsgKeyCreateSuccess).
		WithData(created.Response)
}

// Login 校验账号密码并创建登录态。
func (l *AuthLogic) Login(req *types.LoginReq) *types.BizResult {
	if err := l.validateLoginReq(req); err != nil {
		return types.NewBizResult(codes.ParamError).
			SetI18nMessage(i18n.MsgKeyParamErrorFormat, err.Error()).
			WithError(err)
	}
	cfg := l.svc.CurrentConfig()
	if err := l.checkAuthRateLimit(authRateLimitActionLoginIP, l.ClientIP(), cfg.Auth.LoginRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Username: req.Username,
				Reason:   AuthEventReasonLoginIPRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	if err := l.checkAuthRateLimit(authRateLimitActionLoginUsername, req.Username, cfg.Auth.LoginRateLimit); err != nil {
		if errors.Is(err, ErrAuthRateLimited) {
			l.emitAuthEvent(AuthEventInput{
				Action:   AuthEventActionRateLimited,
				Username: req.Username,
				Reason:   AuthEventReasonLoginUsernameRateLimited,
			})
		}
		return authRateLimitResult(err)
	}
	user, err := model.FindAPIUserByUsername(l.svc.WriteDB(svc.DatabaseMain), req.Username)
	if err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 查询用户[%s]", req.Username).ToBizResult()
	}
	if user == nil {
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			Username: req.Username,
			Reason:   AuthEventReasonInvalidPassword,
		})
		return invalidPasswordResult(errors.Errorf("AuthLogic.Login 用户[%s]不存在", req.Username))
	}
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			UserID:   user.ID,
			Username: user.Username,
			Reason:   AuthEventReasonInvalidPassword,
		})
		return invalidPasswordResult(errors.Errorf("AuthLogic.Login 用户[%s]密码错误", req.Username))
	}
	if user.Status != model.APIUserStatusEnabled {
		l.emitAuthEvent(AuthEventInput{
			Action:   AuthEventActionLoginFailed,
			UserID:   user.ID,
			Username: user.Username,
			Reason:   AuthEventReasonUserDisabled,
		})
		return types.NewBizResult(codes.UserDisabled).
			SetI18nMessage(i18n.MsgKeyUserDisabled).
			WithError(errors.Errorf("AuthLogic.Login 用户[%s]已禁用", req.Username))
	}
	now := time.Now()
	if err = model.UpdateAPIUser(l.svc.WriteDB(svc.DatabaseMain), user.ID, map[string]any{
		"last_login_at": now,
		"last_login_ip": l.ClientIP(),
		"updated_at":    now,
	}); err != nil {
		return types.DBError(i18n.MsgKeyDBError, err, "AuthLogic.Login 更新用户[%s]登录信息", req.Username).ToBizResult()
	}
	user.LastLoginAt = now
	user.LastLoginIP = l.ClientIP()
	user.UpdatedAt = now
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Login 创建用户[%s]会话失败", req.Username).ToBizResult()
	}
	l.clearAuthRateLimit(authRateLimitActionLoginIP, l.ClientIP())
	l.clearAuthRateLimit(authRateLimitActionLoginUsername, req.Username)
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionLoginSuccess,
		UserID:   user.ID,
		Username: user.Username,
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
	user, err := NewUserLogic(l.Context(), l.svc).GetActiveUserForAuth(ctxUser.ID)
	if err != nil {
		if errors.Is(err, ErrAPIUserNotFound) {
			return types.NewBizResult(codes.TokenInvalid).
				SetI18nMessage(i18n.MsgKeyTokenInvalid).
				WithError(wrapLogicError(err, "AuthLogic.Refresh 用户ID[%d]不存在", ctxUser.ID))
		}
		return types.NewBizResult(codes.UserDisabled).
			SetI18nMessage(i18n.MsgKeyUserDisabled).
			WithError(wrapLogicError(err, "AuthLogic.Refresh 用户ID[%d]状态无效", ctxUser.ID))
	}
	oldJTI := ""
	if meta := l.Meta(); meta != nil {
		oldJTI = tokenJTI(meta.AccessToken, l.svc.CurrentConfig().JwtSecret)
	}
	resp, err := l.rotateSession(user, oldJTI)
	if err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Refresh 用户ID[%d]轮换会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionRefreshSuccess,
		UserID:   user.ID,
		Username: user.Username,
		JTI:      tokenJTI(resp.Token, l.svc.CurrentConfig().JwtSecret),
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
	jti := tokenJTI(l.AccessToken(), l.svc.CurrentConfig().JwtSecret)
	if jti == "" {
		return types.NewBizResult(codes.TokenInvalid).
			SetI18nMessage(i18n.MsgKeyTokenInvalid).
			WithError(errors.New("AuthLogic.Logout 当前 token 缺少 jti"))
	}
	if err := l.deleteUserSession(ctxUser.ID, jti); err != nil {
		return types.ServerError(i18n.MsgKeyInternalError, err, "AuthLogic.Logout 用户ID[%d]清理会话", ctxUser.ID).ToBizResult()
	}
	l.emitAuthEvent(AuthEventInput{
		Action:   AuthEventActionLogoutSuccess,
		UserID:   ctxUser.ID,
		Username: ctxUser.Name,
		JTI:      jti,
		Reason:   AuthEventReasonCurrentSessionDeleted,
	})
	return types.NewBizResult(codes.Success).
		SetI18nMessage(i18n.MsgKeyLogoutSuccess)
}

// validateRegisterReq 校验注册请求并规范化用户名。
func (l *AuthLogic) validateRegisterReq(req *types.RegisterReq) error {
	if req == nil {
		return errors.New("请求不能为空")
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < usernameMinLength || len(req.Username) > usernameMaxLength {
		return errors.Errorf("用户名长度必须为 %d-%d 位", usernameMinLength, usernameMaxLength)
	}
	if len(req.Password) < l.passwordMinLength() {
		return errors.Errorf("密码长度不能少于 %d 位", l.passwordMinLength())
	}
	return nil
}

// validateLoginReq 校验登录请求并规范化用户名。
func (l *AuthLogic) validateLoginReq(req *types.LoginReq) error {
	if req == nil {
		return errors.New("请求不能为空")
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return errors.New("用户名和密码不能为空")
	}
	return nil
}

// createdSession 表示已写入 Redis 的新会话。
type createdSession struct {
	Response *types.AuthTokenResp // Response 表示返回给客户端的 token 数据
	JTI      string               // JTI 表示本次新会话的 JWT ID
}

// createSessionWithJTI 生成 JWT、写入 Redis 会话并返回内部 jti。
func (l *AuthLogic) createSessionWithJTI(user *model.APIUser) (*createdSession, error) {
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
	if err = l.Redis().Set(l.Context(), l.userSessionKey(user.ID, jti), token, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
		return nil, errors.Wrapf(err, "写入用户会话失败 user_id=%d jti=%s", user.ID, jti)
	}
	if err = l.addUserSessionIndex(user.ID, jti, sessionExpiresAt, ttlSeconds); err != nil {
		_ = l.deleteUserSession(user.ID, jti)
		return nil, errors.Wrapf(err, "写入用户会话索引失败 user_id=%d jti=%s", user.ID, jti)
	}
	profile := buildUserProfile(user)
	userLogic := NewUserLogic(l.Context(), l.svc)
	// 用户资料缓存只做加速，写入失败不影响已创建的 Redis 会话。
	_ = userLogic.RdsSetJSONValue(userLogic.userProfileKey(user.ID), profile, l.profileCacheTTL())
	return &createdSession{
		JTI: jti,
		Response: &types.AuthTokenResp{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      profile,
		},
	}, nil
}

// rotateSession 创建新会话后删除旧会话，删除失败时回滚新会话。
func (l *AuthLogic) rotateSession(user *model.APIUser, oldJTI string) (*types.AuthTokenResp, error) {
	oldJTI = strings.TrimSpace(oldJTI)
	if user == nil {
		return nil, errors.New("用户为空")
	}
	if oldJTI == "" {
		return nil, errors.New("旧会话 jti 为空")
	}
	created, err := l.createSessionWithJTI(user)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if err := l.deleteUserSession(user.ID, oldJTI); err != nil {
		_ = l.deleteUserSession(user.ID, created.JTI)
		return nil, errors.Wrapf(err, "删除旧用户会话失败 user_id=%d old_jti=%s", user.ID, oldJTI)
	}
	return created.Response, nil
}

// generateJWT 生成包含用户、站点和 jti 信息的访问令牌。
func (l *AuthLogic) generateJWT(user *model.APIUser, jti string) (string, int64, error) {
	cfg := l.svc.CurrentConfig()
	now := time.Now()
	expiresAt := now.Add(time.Duration(cfg.JwtExpiresIn) * time.Second).Unix()
	claims := jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"jti":      jti,
		"iss":      cfg.Auth.Issuer,
		"app_id":   userSessionAppID(cfg.AppID),
		"iat":      now.Unix(),
		"exp":      expiresAt,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.JwtSecret))
	return tokenString, expiresAt, errors.Tag(err)
}

// userSessionKey 生成当前站点下的用户会话 Redis Key。
func (l *AuthLogic) userSessionKey(userID int64, jti string) string {
	cfg := l.svc.CurrentConfig()
	return fmt.Sprintf(keys.UserSession, userSessionAppID(cfg.AppID), userID, strings.TrimSpace(jti))
}

// userSessionIndexKey 生成当前站点下的用户会话 jti 索引 Redis Key。
func (l *AuthLogic) userSessionIndexKey(userID int64) string {
	cfg := l.svc.CurrentConfig()
	return fmt.Sprintf(keys.UserSessionIndex, userSessionAppID(cfg.AppID), userID)
}

// userSessionAppID 返回用户会话使用的站点命名空间。
func userSessionAppID(appID string) string {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return "default"
	}
	return appID
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
	jtis, err := l.Redis().ZRange(l.Context(), indexKey, 0, -1).Result()
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
	if err := l.Redis().ZAdd(l.Context(), indexKey, redis.Z{
		Score:  float64(expiresAt),
		Member: jti,
	}).Err(); err != nil {
		return errors.Wrapf(err, "写入用户会话索引失败 user_id=%d jti=%s", userID, jti)
	}
	if ttlSeconds <= 0 {
		ttlSeconds = l.sessionTTL()
	}
	if err := l.Redis().Expire(l.Context(), indexKey, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
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
	return errors.Tag(l.Redis().ZRem(l.Context(), l.userSessionIndexKey(userID), jti).Err())
}

// pruneExpiredUserSessionIndex 删除已自然过期的 jti 索引成员。
func (l *AuthLogic) pruneExpiredUserSessionIndex(userID int64) error {
	if userID <= 0 || l.Redis() == nil {
		return nil
	}
	now := time.Now().Unix()
	return errors.Tag(l.Redis().ZRemRangeByScore(l.Context(), l.userSessionIndexKey(userID), "-inf", fmt.Sprintf("%d", now)).Err())
}

// sessionTTL 返回 Redis 会话 TTL，不超过 JWT 过期时间。
func (l *AuthLogic) sessionTTL() int64 {
	cfg := l.svc.CurrentConfig()
	jwtTTL := cfg.JwtExpiresIn
	if jwtTTL <= 0 {
		jwtTTL = 86400
	}
	if cfg.Auth.SessionTTLSeconds > 0 && cfg.Auth.SessionTTLSeconds < jwtTTL {
		return cfg.Auth.SessionTTLSeconds
	}
	return jwtTTL
}

// profileCacheTTL 返回用户资料缓存 TTL，未配置时使用 5 分钟。
func (l *AuthLogic) profileCacheTTL() int64 {
	cfg := l.svc.CurrentConfig()
	if cfg.Auth.ProfileCacheTTLSeconds > 0 {
		return cfg.Auth.ProfileCacheTTLSeconds
	}
	return 300
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
	if err := l.Redis().Get(l.Context(), lockKey).Err(); err == nil {
		return ErrAuthRateLimited
	} else if err != nil && !errors.Is(err, redis.Nil) {
		return errors.Wrapf(err, "读取认证限流锁失败 action=%s", action)
	}
	count, err := l.Redis().Incr(l.Context(), countKey).Result()
	if err != nil {
		return errors.Wrapf(err, "写入认证限流计数失败 action=%s", action)
	}
	if count == 1 {
		if err := l.Redis().Expire(l.Context(), countKey, time.Duration(cfg.WindowSeconds)*time.Second).Err(); err != nil {
			return errors.Wrapf(err, "设置认证限流窗口失败 action=%s", action)
		}
	}
	if count > int64(cfg.MaxAttempts) {
		if err := l.Redis().Set(l.Context(), lockKey, "1", time.Duration(cfg.LockSeconds)*time.Second).Err(); err != nil {
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
	subjectHash := utils.Md5(subject)
	appID := authRateLimitAppID(l.svc)
	return fmt.Sprintf(keys.AuthRateLimitCount, appID, action, subjectHash),
		fmt.Sprintf(keys.AuthRateLimitLock, appID, action, subjectHash)
}

// authRateLimitAppID 返回认证限流使用的站点命名空间。
func authRateLimitAppID(svcCtx *svc.ServiceContext) string {
	appID := ""
	if svcCtx != nil {
		appID = strings.TrimSpace(svcCtx.CurrentConfig().AppID)
	}
	if appID == "" {
		return "default"
	}
	return appID
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
	cfg := l.svc.CurrentConfig()
	if cfg.Auth.PasswordMinLength > 0 {
		return cfg.Auth.PasswordMinLength
	}
	return 8
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
