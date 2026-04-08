package handler

import (
	"net/http"

	codes "gozero_api/common/codes"
	"gozero_api/helper"
	"gozero_api/internal/infra/loggerx"
	"gozero_api/internal/logic"
	"gozero_api/internal/requestctx"
	"gozero_api/internal/svc"
)

// HealthHandler 提供兼容健康检查接口，行为等同于 LiveHandler。
func HealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return LiveHandler(svcCtx)
}

// LiveHandler 提供进程存活检查。
func LiveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(HealthLive.Alias))
		resp := logic.NewHealthLogic(r.Context(), svcCtx).Liveness()
		helper.NewJsonResp(r.Context(), w).SetCode(codes.OK).Success(resp)
	}
}

// ReadyHandler 提供依赖就绪检查。
func ReadyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(HealthReady.Alias))
		resp, err := logic.NewHealthLogic(r.Context(), svcCtx).Readiness(r.Context())
		if err != nil {
			loggerx.Errorw(r.Context(), "健康检查 依赖未就绪", err)
			helper.NewJsonResp(r.Context(), w).
				SetHttpStatus(http.StatusServiceUnavailable).
				SetCode(codes.DependencyUnavailable).
				SetError(err).
				Fail("", resp)
			return
		}
		helper.NewJsonResp(r.Context(), w).SetCode(codes.OK).Success(resp)
	}
}
