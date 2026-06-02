package config

import (
	"net/http"

	"api/internal/handler/shared"
	configlogic "api/internal/logic/config"
	"api/internal/svc"
	"api/internal/types"
)

// ConfigReloadStatusHandler 查询配置热加载运行状态。
func ConfigReloadStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := configlogic.NewSystemLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.ConfigReloadStatus())
	}
}

// ConfigReloadItemsHandler 查询当前运行态配置项。
func ConfigReloadItemsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return shared.RespHandler[types.ConfigItemQueryReq](func(r *http.Request, svcCtx *svc.ServiceContext, req *types.ConfigItemQueryReq) *types.BizResult {
		l := configlogic.NewSystemLogic(r.Context(), svcCtx)
		return l.ConfigReloadItems(req)
	})(svcCtx)
}

// RunConfigReloadHandler 手动触发一次配置热加载。
func RunConfigReloadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := configlogic.NewSystemLogic(r.Context(), svcCtx)
		shared.WriteBizResponse(w, r, l.RunConfigReload())
	}
}
