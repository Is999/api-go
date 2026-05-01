package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	keys "api/common/rediskeys"
	"api/common/runtimecfg"
	"api/internal/config"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/golang-jwt/jwt/v4"
	"github.com/redis/go-redis/v9"
)

// token 校验内部错误哨兵，handler 按错误类型映射业务码。
var (
	// errMissingBearerToken 表示 Authorization 头缺少 Bearer token。
	errMissingBearerToken = errors.New("缺少 Bearer Token")
	// errInvalidToken 表示 token 签名、结构或声明无效。
	errInvalidToken = errors.New("Token 无效")
	// errTokenExpired 表示 token 已超过 exp 时间。
	errTokenExpired = errors.New("Token 已过期")
	// errSessionExpired 表示服务端 Redis 会话不存在或已过期。
	errSessionExpired = errors.New("会话已过期")
)

// UserTokenIdentity 表示通过 JWT 和 Redis 会话校验后的用户身份。
type UserTokenIdentity struct {
	UserID    int64  // 用户 ID，来自 JWT sub
	UserName  string // 用户名，来自 JWT username
	Token     string // 当前请求携带的原始 token
	JTI       string // JWT ID
	ExpiresAt int64  // token 过期时间戳，单位秒
}

// bearerToken 从 Authorization 头中提取 Bearer token。
func bearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return "", errMissingBearerToken
	}
	token := strings.TrimSpace(header[len("Bearer "):])
	if token == "" {
		return "", errMissingBearerToken
	}
	return token, nil
}

// VerifyUserToken 统一校验前台 JWT，并按需校验 Redis 中的登录会话。
func VerifyUserToken(ctx context.Context, svcCtx *svc.ServiceContext, tokenString string, requireSession bool) (*UserTokenIdentity, error) {
	cfg := config.Config{}
	if svcCtx != nil {
		cfg = svcCtx.CurrentConfig()
	}
	if svcCtx == nil || strings.TrimSpace(cfg.JwtSecret) == "" || strings.TrimSpace(tokenString) == "" {
		return nil, errInvalidToken
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Wrap(errInvalidToken, "签名算法不匹配")
		}
		return []byte(cfg.JwtSecret), nil
	})
	if err != nil || !token.Valid {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errTokenExpired
		}
		return nil, errInvalidToken
	}

	userID, ok := claims["sub"].(float64)
	if !ok || userID <= 0 {
		return nil, errInvalidToken
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, errInvalidToken
	}
	if int64(exp) < time.Now().Unix() {
		return nil, errTokenExpired
	}
	jti := strings.TrimSpace(fmt.Sprint(claims["jti"]))
	if jti == "" {
		return nil, errInvalidToken
	}
	if !tokenAppIDMatches(cfg.AppID, strings.TrimSpace(fmt.Sprint(claims["app_id"]))) {
		return nil, errInvalidToken
	}
	if strings.TrimSpace(cfg.AppID) != runtimecfg.AppID() {
		return nil, errInvalidToken
	}
	identity := &UserTokenIdentity{
		UserID:    int64(userID),
		UserName:  strings.TrimSpace(fmt.Sprint(claims["username"])),
		Token:     tokenString,
		JTI:       jti,
		ExpiresAt: int64(exp),
	}
	if !requireSession {
		return identity, nil
	}
	if svcCtx.Rds == nil {
		return identity, errInvalidToken
	}
	sessionKey := UserSessionKey(identity.UserID, identity.JTI)
	savedToken, err := svcCtx.Rds.Get(ctx, sessionKey).Result()
	if errors.Is(err, redis.Nil) {
		return identity, errSessionExpired
	}
	if err != nil {
		return identity, errInvalidToken
	}
	if strings.TrimSpace(savedToken) != tokenString {
		return identity, errInvalidToken
	}
	_ = syncUserSessionIndex(ctx, svcCtx.Rds, identity.UserID, identity.JTI, identity.ExpiresAt, sessionKey)
	return identity, nil
}

// VerifyUserTokenFromRequest 从 HTTP 请求中提取并校验前台用户 token。
func VerifyUserTokenFromRequest(ctx context.Context, svcCtx *svc.ServiceContext, r *http.Request, requireSession bool) (*UserTokenIdentity, error) {
	tokenString, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		return nil, errors.Tag(err)
	}
	return VerifyUserToken(ctx, svcCtx, tokenString, requireSession)
}

// UserSessionKey 生成前台用户会话缓存键。
func UserSessionKey(userID int64, jti string) string {
	return keys.WithPrefix(fmt.Sprintf(keys.UserSession, userID, strings.TrimSpace(jti)))
}

// UserSessionIndexKey 生成前台用户会话 jti 索引键。
func UserSessionIndexKey(userID int64) string {
	return keys.WithPrefix(fmt.Sprintf(keys.UserSessionIndex, userID))
}

// syncUserSessionIndex 在鉴权成功后补齐用户 jti 索引，保证会话批量失效可精确命中。
func syncUserSessionIndex(ctx context.Context, rds redis.UniversalClient, userID int64, jti string, expiresAt int64, sessionKey string) error {
	jti = strings.TrimSpace(jti)
	if rds == nil || userID <= 0 || jti == "" {
		return nil
	}
	now := time.Now()
	expires := time.Unix(expiresAt, 0)
	ttl := time.Until(expires)
	if strings.TrimSpace(sessionKey) != "" {
		if sessionTTL, err := rds.TTL(ctx, sessionKey).Result(); err == nil && sessionTTL > 0 && sessionTTL < ttl {
			ttl = sessionTTL
		}
	}
	if ttl <= 0 {
		return nil
	}
	indexKey := UserSessionIndexKey(userID)
	_ = rds.ZRemRangeByScore(ctx, indexKey, "-inf", fmt.Sprintf("%d", now.Unix())).Err()
	if err := rds.ZAdd(ctx, indexKey, redis.Z{
		Score:  float64(expiresAt),
		Member: jti,
	}).Err(); err != nil {
		return errors.Tag(err)
	}
	if indexTTL, err := rds.TTL(ctx, indexKey).Result(); err == nil && indexTTL > ttl {
		ttl = indexTTL
	}
	return errors.Tag(rds.Expire(ctx, indexKey, ttl).Err())
}

// tokenAppIDMatches 判断 token 中的 app_id 是否匹配当前服务命名空间。
func tokenAppIDMatches(configAppID string, claimAppID string) bool {
	expected := strings.TrimSpace(configAppID)
	claimAppID = strings.TrimSpace(claimAppID)
	return expected != "" && claimAppID != "" && claimAppID == expected
}
