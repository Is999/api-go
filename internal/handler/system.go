package handler

import (
	"net/http"

	"gozero_api/internal/logic"
	"gozero_api/internal/svc"
)

// ConfigReloadStatusHandler 查询配置热加载运行状态。
func ConfigReloadStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewSystemLogic(r.Context(), svcCtx)
		writeBizResponse(w, r, l.ConfigReloadStatus())
	}
}

// RunConfigReloadHandler 手动触发一次配置热加载。
func RunConfigReloadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewSystemLogic(r.Context(), svcCtx)
		writeBizResponse(w, r, l.RunConfigReload())
	}
}
