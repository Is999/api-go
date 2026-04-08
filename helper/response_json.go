package helper

import (
	"context"
	"net/http"
	"strings"

	codes "gozero_api/common/codes"
	i18n "gozero_api/common/i18n"
	"gozero_api/internal/requestctx"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// 统一响应状态常量，兼容前端已有成功/失败判断。
const (
	RespCodeUndefined = 0 // 未定义状态
	RespCodeSuccess   = 1 // 成功
	RespCodeFail      = 2 // 失败
)

// ResponseJSON 标准响应 JSON 结构。
type ResponseJSON struct {
	Status  bool   `json:"status"`            // 状态: true 成功, false 失败
	Code    int    `json:"code"`              // 业务状态码
	Message string `json:"message,omitempty"` // 响应消息
	Data    any    `json:"data,omitempty"`    // 响应数据
	TraceID string `json:"traceId,omitempty"` // 链路追踪 ID
	SpanID  string `json:"spanId,omitempty"`  // 当前服务 span ID
}

// JsonResp 用链式写法封装统一响应。
type JsonResp struct {
	ctx        context.Context     // 当前响应绑定的请求上下文
	write      http.ResponseWriter // HTTP 响应写入器
	httpStatus *int                // 显式 HTTP 状态码，nil 时按业务码推导
	code       *int                // 显式业务码，nil 时使用默认成功/失败码
	message    *string             // 显式消息 key 或最终文案
	err        error               // 内部错误对象，仅写入日志和 trace
}

// NewJsonResp 创建响应写入器。
func NewJsonResp(ctx context.Context, w http.ResponseWriter) *JsonResp {
	return &JsonResp{
		ctx:   ctx,
		write: w,
	}
}

// SetHttpStatus 设置本次响应的 HTTP 状态码。
func (r *JsonResp) SetHttpStatus(status int) *JsonResp {
	r.httpStatus = &status
	return r
}

// SetCode 设置业务状态码。
func (r *JsonResp) SetCode(code int) *JsonResp {
	r.code = &code
	return r
}

// SetMessage 设置响应消息，可传多语言 key 或最终展示文案。
func (r *JsonResp) SetMessage(message string) *JsonResp {
	r.message = &message
	return r
}

// SetError 设置仅供内部日志、审计和 trace 使用的错误对象。
func (r *JsonResp) SetError(err error) *JsonResp {
	r.err = err
	return r
}

// Success 构造成功响应，并把请求结果同步写入 request meta。
func (r *JsonResp) Success(data any) {
	locale := responseLocale(r.ctx)
	code := RespCodeSuccess
	if r.code != nil {
		code = *r.code
	}
	message := i18n.MessageByCode(code, locale)
	if r.message != nil {
		message = i18n.MessageByKey(*r.message, locale)
	}
	requestctx.SetErrorResponse(r.ctx, http.StatusOK, code, message, nil, "")

	response := ResponseJSON{
		Status:  true,
		Code:    code,
		Message: message,
		Data:    data,
	}
	attachTraceToResponse(r.ctx, &response)
	httpx.WriteJsonCtx(r.ctx, r.write, http.StatusOK, response)
}

// Fail 构造失败响应，并同步 request meta 供日志与 trace 使用。
func (r *JsonResp) Fail(message string, data ...any) {
	locale := responseLocale(r.ctx)
	code := RespCodeFail
	if r.code != nil {
		code = *r.code
	}
	if message == "" {
		message = i18n.MessageByCode(code, locale)
	} else {
		message = i18n.MessageByKey(message, locale)
	}
	response := ResponseJSON{
		Status:  false,
		Code:    code,
		Message: message,
		Data: func() any {
			if len(data) > 0 {
				return data[0]
			}
			return nil
		}(),
	}
	attachTraceToResponse(r.ctx, &response)
	httpStatus := codes.HTTPStatus(code)
	if r.httpStatus != nil {
		httpStatus = *r.httpStatus
	}
	requestctx.SetErrorResponse(r.ctx, httpStatus, code, message, r.err, internalErrorSummary(r.err))
	httpx.WriteJsonCtx(r.ctx, r.write, httpStatus, response)
}

// Write 允许业务显式指定成功/失败标志。
func (r *JsonResp) Write(success bool, data ...any) {
	locale := responseLocale(r.ctx)
	code := RespCodeUndefined
	if r.code != nil {
		code = *r.code
	}
	message := i18n.MessageByCode(code, locale)
	if r.message != nil {
		message = i18n.MessageByKey(*r.message, locale)
	}
	response := ResponseJSON{
		Status:  success,
		Code:    code,
		Message: message,
		Data: func() any {
			if len(data) > 0 {
				return data[0]
			}
			return nil
		}(),
	}
	attachTraceToResponse(r.ctx, &response)
	httpStatus := codes.HTTPStatus(code)
	if r.httpStatus != nil {
		httpStatus = *r.httpStatus
	}
	if success {
		requestctx.SetErrorResponse(r.ctx, httpStatus, code, message, nil, "")
	} else {
		requestctx.SetErrorResponse(r.ctx, httpStatus, code, message, r.err, internalErrorSummary(r.err))
	}
	httpx.WriteJsonCtx(r.ctx, r.write, httpStatus, response)
}

// attachTraceToResponse 把链路字段回填到响应体。
func attachTraceToResponse(ctx context.Context, response *ResponseJSON) {
	if response == nil {
		return
	}
	meta := requestctx.FromContext(ctx)
	if meta == nil {
		return
	}
	response.TraceID = strings.TrimSpace(meta.TraceID)
	response.SpanID = strings.TrimSpace(meta.SpanID)
}

// responseLocale 统一读取响应语言，缺省时回退中文。
func responseLocale(ctx context.Context) string {
	if meta := requestctx.FromContext(ctx); meta != nil && meta.Locale != "" {
		return meta.Locale
	}
	return i18n.LocaleZHCN
}

// internalErrorSummary 生成仅供内部日志和 trace 使用的错误摘要。
func internalErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
