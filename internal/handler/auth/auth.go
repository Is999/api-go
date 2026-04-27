package auth

import (
	"net/http"

	"api/internal/handler/shared"
	authlogic "api/internal/logic/auth"
	"api/internal/svc"
	"api/internal/types"
)

// RegisterHandler 处理前台用户注册。
func RegisterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return shared.RespHandler(func(r *http.Request, svcCtx *svc.ServiceContext, req *types.RegisterReq) *types.BizResult {
		return authlogic.NewAuthLogic(r.Context(), svcCtx).Register(req)
	})(svcCtx)
}

// LoginHandler 处理前台用户登录。
func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return shared.RespHandler(func(r *http.Request, svcCtx *svc.ServiceContext, req *types.LoginReq) *types.BizResult {
		return authlogic.NewAuthLogic(r.Context(), svcCtx).Login(req)
	})(svcCtx)
}

// RefreshHandler 处理访问令牌刷新。
func RefreshHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := authlogic.NewAuthLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.Refresh())
	}
}

// LogoutHandler 处理前台用户退出登录。
func LogoutHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := authlogic.NewAuthLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.Logout())
	}
}
