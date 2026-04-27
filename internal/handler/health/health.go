package health

import (
	"net/http"

	codes "api/common/codes"
	"api/helper"
	"api/internal/handler/shared"
	"api/internal/infra/loggerx"
	healthlogic "api/internal/logic/health"
	"api/internal/requestctx"
	"api/internal/svc"
)

// LiveHandler 提供进程存活检查。
func LiveHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(shared.HealthLive.Alias))
		resp := healthlogic.NewHealthLogic(r.Context(), svcCtx).Liveness()
		helper.NewJsonResp(r.Context(), w).SetCode(codes.OK).Success(resp)
	}
}

// ReadyHandler 提供依赖就绪检查。
func ReadyHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestctx.SetRoute(r.Context(), string(shared.HealthReady.Alias))
		resp, err := healthlogic.NewHealthLogic(r.Context(), svcCtx).Readiness(r.Context())
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
