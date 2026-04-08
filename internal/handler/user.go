package handler

import (
	"net/http"

	"gozero_api/internal/logic"
	"gozero_api/internal/svc"
)

// UserProfileHandler 获取当前用户资料。
func UserProfileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewUserLogic(r.Context(), svcCtx)
		writeBizResponse(w, r, l.Profile())
	}
}
