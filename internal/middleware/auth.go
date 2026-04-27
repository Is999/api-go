package middleware

import (
	"context"
	"net/http"

	codes "api/common/codes"
	i18n "api/common/i18n"
	"api/helper"
	"api/internal/infra/loggerx"
	authlogic "api/internal/logic/auth"
	userlogic "api/internal/logic/user"
	"api/internal/requestctx"
	"api/internal/svc"

	"github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

// RouteAlias 是路由在日志、权限和排障体系中的稳定标识。
type RouteAlias string

// 鉴权中间件路由别名特殊值。
const (
	// Ignore 表示该路由跳过业务路由别名写入。
	Ignore RouteAlias = "ignore"
)

// AuthMiddleware 负责 JWT 鉴权、Redis session 校验以及请求元数据补全。
type AuthMiddleware struct {
	svc       *svc.ServiceContext  // 鉴权依赖的服务上下文
	crypto    *CryptoMiddleware    // 请求解密与响应加密中间件
	signature *SignatureMiddleware // 请求验签与响应签名中间件
}

// NewAuthMiddleware 创建鉴权中间件实例。
func NewAuthMiddleware(svcCtx *svc.ServiceContext) *AuthMiddleware {
	return &AuthMiddleware{
		svc:       svcCtx,
		crypto:    NewCryptoMiddleware(svcCtx),
		signature: NewSignatureMiddleware(svcCtx),
	}
}

// PublicHandle 为未登录接口挂载加密与签名中间件，但不执行 JWT 鉴权。
func (m *AuthMiddleware) PublicHandle(next http.HandlerFunc, alias RouteAlias) http.HandlerFunc {
	handler := next
	if m.signature != nil {
		handler = m.signature.Handle(handler, alias)
	}
	if m.crypto != nil {
		handler = m.crypto.Handle(handler)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, bindRequestMeta(r, alias))
	}
}

// Handle 负责鉴权并补齐当前请求的用户信息。
func (m *AuthMiddleware) Handle(next http.HandlerFunc, alias RouteAlias) http.HandlerFunc {
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := requestctx.New(r.Context())
		requestctx.SetRequest(ctx, r.Method, r.URL.Path, utils.ClientIP(r))
		if alias != "" && alias != Ignore {
			requestctx.SetRoute(ctx, string(alias))
		}

		failUnauthorized := func(code int, messageKey string, err error, reason string, identity *UserTokenIdentity) {
			m.emitAuthFailureEvent(ctx, reason, identity)
			resp := helper.NewJsonResp(ctx, w).
				SetHttpStatus(http.StatusUnauthorized).
				SetCode(code)
			if err != nil {
				resp = resp.SetError(err)
			}
			resp.Fail(messageKey)
		}

		identity, err := VerifyUserTokenFromRequest(ctx, m.svc, r, true)
		switch {
		case errors.Is(err, errMissingBearerToken):
			failUnauthorized(codes.Unauthorized, i18n.MsgKeyUnauthorizedText, err, authlogic.AuthEventReasonMissingBearer, nil)
			return
		case errors.Is(err, errTokenExpired):
			failUnauthorized(codes.TokenExpired, i18n.MsgKeyTokenExpired, err, authlogic.AuthEventReasonTokenExpired, identity)
			return
		case errors.Is(err, errSessionExpired):
			failUnauthorized(codes.SessionExpired, i18n.MsgKeySessionExpired, err, authlogic.AuthEventReasonSessionExpired, identity)
			return
		case err != nil:
			failUnauthorized(codes.TokenInvalid, i18n.MsgKeyTokenInvalid, err, authlogic.AuthEventReasonTokenInvalid, identity)
			return
		}

		user, err := userlogic.NewUserLogic(ctx, m.svc).GetActiveUserForAuth(identity.UserID)
		if err != nil {
			if errors.Is(err, userlogic.ErrAPIUserDisabled) {
				failUnauthorized(codes.UserDisabled, i18n.MsgKeyUserDisabled, err, authlogic.AuthEventReasonUserDisabled, identity)
				return
			}
			reason := authlogic.AuthEventReasonTokenInvalid
			if errors.Is(err, userlogic.ErrAPIUserNotFound) {
				reason = authlogic.AuthEventReasonUserNotFound
			}
			failUnauthorized(codes.TokenInvalid, i18n.MsgKeyTokenInvalid, err, reason, identity)
			return
		}

		requestctx.SetAccessToken(ctx, identity.Token)
		requestctx.SetUser(ctx, identity.UserID, user.Username, utils.ClientIP(r))
		ctx = loggerx.BindContext(ctx)
		next(w, r.WithContext(ctx))
	}
	return m.PublicHandle(handler, alias)
}

// emitAuthFailureEvent 投递登录态鉴权失败事件，Collector 不可用时不影响响应。
func (m *AuthMiddleware) emitAuthFailureEvent(ctx context.Context, reason string, identity *UserTokenIdentity) {
	if m == nil || m.svc == nil {
		return
	}
	input := authlogic.AuthEventInput{
		Action: authlogic.AuthEventActionAuthFailed,
		Reason: reason,
	}
	if identity != nil {
		input.UserID = identity.UserID
		input.Username = identity.UserName
		input.JTI = identity.JTI
	}
	authlogic.RecordAuthEvent(ctx, m.svc, input)
}

// bindRequestMeta 为公开路由补齐请求元数据和稳定路由别名。
func bindRequestMeta(r *http.Request, alias RouteAlias) *http.Request {
	if r == nil {
		return r
	}
	ctx, _ := requestctx.New(r.Context())
	requestctx.SetRequest(ctx, r.Method, r.URL.Path, utils.ClientIP(r))
	if alias != "" && alias != Ignore {
		requestctx.SetRoute(ctx, string(alias))
	}
	return r.WithContext(ctx)
}
