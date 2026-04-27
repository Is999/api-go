package shared

import (
	"net/http"

	"github.com/Is999/go-utils/errors"

	codes "api/common/codes"
	"api/helper"
	"api/internal/infra/loggerx"
	"api/internal/svc"
	"api/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// handlerFunc 是无泛型请求解析后的统一业务处理函数。
type handlerFunc func(r *http.Request) *types.BizResult

// RespExec 定义标准 handler 执行函数。
type RespExec[Req any] func(*http.Request, *svc.ServiceContext, *Req) *types.BizResult

// RespHandler 泛型封装，简化普通接口 handler 模板代码。
func RespHandler[Req any](exec RespExec[Req]) func(*svc.ServiceContext) http.HandlerFunc {
	return func(sCtx *svc.ServiceContext) http.HandlerFunc {
		return respHandler(func(r *http.Request) *types.BizResult {
			var req Req
			if err := httpx.Parse(r, &req); err != nil {
				return paramErrorResult(err)
			}
			resp := exec(r, sCtx, &req)
			if resp == nil {
				return types.NewBizResult(codes.ServerError).WithError(errors.New("业务响应为空"))
			}
			resp.WithReq(&req)
			return resp
		})
	}
}

// respHandler 把业务结果写入统一 JSON 响应。
func respHandler(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteBizResponse(w, r, fn(r))
	}
}

// paramErrorResult 把请求解析错误转换为统一参数错误响应。
func paramErrorResult(err error) *types.BizResult {
	return types.ParamErrorResult(err)
}

// WriteBizResponse 在 handler 最外层统一输出响应和错误日志。
func WriteBizResponse(w http.ResponseWriter, r *http.Request, resp *types.BizResult) {
	if resp == nil {
		resp = types.NewBizResult(codes.ServerError).WithError(errors.New("业务响应为空"))
	}
	message := resp.ResolveMessage(r.Context())
	if resp.IsFailure() {
		if resp.Error != nil && !errors.Is(resp.Error, types.Nil) {
			loggerx.Errorw(r.Context(), "请求 业务处理失败", resp.Error)
		}
		jsonResp := helper.NewJsonResp(r.Context(), w).SetCode(resp.Code)
		if resp.Error != nil && !errors.Is(resp.Error, types.Nil) {
			jsonResp = jsonResp.SetError(resp.Error)
		}
		jsonResp.Fail(message)
		return
	}
	helper.NewJsonResp(r.Context(), w).SetCode(resp.Code).SetMessage(message).Success(resp.Data)
}
