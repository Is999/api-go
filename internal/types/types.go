//lint:file-ignore SA5008 ignore go-zero optional tag

package types

import (
	"context"

	codes "api/common/codes"
	i18n "api/common/i18n"
	"api/internal/requestctx"

	"github.com/Is999/go-utils/errors"
)

// Nil 表示一个显式的空业务错误占位值。
const Nil = BizError("Biz: nil")

// BizError 通用业务错误类型。
type BizError string

// Error 返回业务错误文本。
func (e BizError) Error() string { return string(e) }

// BizResult 通用响应结构体。
type BizResult struct {
	Code        int    `json:"-"` // 响应代码
	MessageKey  string `json:"-"` // 国际化消息 key
	MessageArgs []any  `json:"-"` // 国际化消息参数
	Error       error  `json:"-"` // 错误信息
	Req         any    `json:"-"` // 请求参数
	Data        any    `json:"-"` // 响应数据
}

// NewBizResult 创建业务响应基础对象。
func NewBizResult(code int) *BizResult {
	return &BizResult{Code: code}
}

// IsSuccess 判断当前业务结果是否可视为成功。
func (r *BizResult) IsSuccess() bool {
	if r == nil {
		return false
	}
	return codes.IsSuccess(r.Code) && r.Error == nil
}

// IsFailure 判断当前业务结果是否为失败。
func (r *BizResult) IsFailure() bool {
	return !r.IsSuccess()
}

// SetI18nMessage 设置国际化消息 key 与参数。
func (r *BizResult) SetI18nMessage(key string, args ...any) *BizResult {
	if r == nil {
		return r
	}
	r.MessageKey = key
	r.MessageArgs = args
	return r
}

// WithError 设置业务错误对象，供统一日志和失败响应分支使用。
func (r *BizResult) WithError(err error) *BizResult {
	if r == nil {
		return r
	}
	if err == nil || errors.Is(err, Nil) {
		r.Error = err
		return r
	}
	r.Error = errors.Tag(err)
	return r
}

// WithReq 设置原始请求对象。
func (r *BizResult) WithReq(req any) *BizResult {
	if r == nil {
		return r
	}
	r.Req = req
	return r
}

// WithData 设置响应数据负载。
func (r *BizResult) WithData(data any) *BizResult {
	if r == nil {
		return r
	}
	r.Data = data
	return r
}

// ParamErrorResultWithCode 统一构造参数错误响应，并挂上国际化模板消息。
func ParamErrorResultWithCode(code int, err error) *BizResult {
	if code <= 0 {
		code = codes.ParamError
	}
	if err == nil {
		return NewBizResult(code).WithError(Nil).SetI18nMessage(i18n.MsgKeyParamError)
	}
	message := err.Error()
	return NewBizResult(code).WithError(Nil).SetI18nMessage(i18n.MsgKeyParamErrorFormat, message)
}

// ParamErrorResult 统一构造参数错误响应，并挂上国际化模板消息。
func ParamErrorResult(err error) *BizResult {
	return ParamErrorResultWithCode(codes.ParamError, err)
}

// ResolveMessage 按“MessageKey > Code 默认文案”的优先级解析最终响应文案。
func (r *BizResult) ResolveMessage(ctx context.Context) string {
	if r == nil {
		return ""
	}
	locale := i18n.LocaleZHCN
	if meta := requestctx.FromContext(ctx); meta != nil && meta.Locale != "" {
		locale = meta.Locale
	}
	if r.MessageKey != "" {
		return i18n.MessageByKey(r.MessageKey, locale, r.MessageArgs...)
	}
	return i18n.MessageByCode(r.Code, locale)
}
