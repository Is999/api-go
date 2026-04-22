package user

import (
	"net/http"

	"api/internal/handler/shared"
	userlogic "api/internal/logic/user"
	"api/internal/svc"
)

// UserProfileHandler 获取当前用户资料。
func UserProfileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := userlogic.NewUserLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.Profile())
	}
}
