package config

import (
	"net/http"

	"api/internal/handler/shared"
	configlogic "api/internal/logic/config"
	"api/internal/svc"
)

// ConfigReloadStatusHandler 查询配置热加载运行状态。
func ConfigReloadStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := configlogic.NewSystemLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.ConfigReloadStatus())
	}
}

// RunConfigReloadHandler 手动触发一次配置热加载。
func RunConfigReloadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := configlogic.NewSystemLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.RunConfigReload())
	}
}
