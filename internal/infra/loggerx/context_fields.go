package loggerx

import (
	"api/internal/requestctx"
	"context"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel/attribute"
)

// FieldsFromContext 从请求上下文提取统一日志字段。
func FieldsFromContext(ctx context.Context) []logx.LogField {
	return FieldsFromMeta(requestctx.FromContext(ctx))
}

// FieldsFromMeta 把请求元数据转换成结构化日志字段。
func FieldsFromMeta(meta *requestctx.Meta) []logx.LogField {
	if meta == nil {
		return nil
	}
	fields := make([]logx.LogField, 0, 24)
	if meta.TraceID != "" {
		fields = append(fields, logx.Field(fieldTraceID, meta.TraceID))
	}
	if meta.SpanID != "" {
		fields = append(fields, logx.Field(fieldSpanID, meta.SpanID))
	}
	if meta.Route != "" {
		fields = append(fields, logx.Field(fieldRoute, meta.Route))
	}
	if meta.Method != "" {
		fields = append(fields, logx.Field(fieldHTTPMethod, meta.Method))
	}
	if meta.Path != "" {
		fields = append(fields, logx.Field(fieldPath, meta.Path))
	}
	if meta.Locale != "" {
		fields = append(fields, logx.Field(fieldLocale, meta.Locale))
	}
	if meta.ClientIP != "" {
		fields = append(fields, logx.Field(fieldIP, meta.ClientIP))
	}
	if meta.UserID > 0 {
		fields = append(fields,
			logx.Field(fieldUID, meta.UserID),
			logx.Field(fieldUserID, meta.UserID),
		)
	}
	if meta.UserName != "" {
		fields = append(fields, logx.Field(fieldUserName, meta.UserName))
	}
	if meta.Node != "" {
		fields = append(fields, logx.Field(fieldNode, meta.Node))
	}
	if meta.Mode != "" {
		fields = append(fields, logx.Field(fieldMode, meta.Mode))
	}
	if meta.HTTPStatus > 0 {
		fields = append(fields, logx.Field(fieldHTTPStatus, meta.HTTPStatus))
	}
	if meta.BizCode > 0 {
		fields = append(fields, logx.Field(fieldBizCode, meta.BizCode))
	}
	if meta.BizMessage != "" {
		fields = append(fields, logx.Field(fieldBizMessage, meta.BizMessage))
	}
	if meta.ErrorMessage != "" {
		fields = append(fields, logx.Field(fieldErrorMsg, meta.ErrorMessage))
	}
	if meta.TaskID != "" {
		fields = append(fields, logx.Field(fieldTaskID, meta.TaskID))
	}
	if meta.WorkflowID != "" {
		fields = append(fields, logx.Field(fieldWorkflowID, meta.WorkflowID))
	}
	if meta.WorkflowNode != "" {
		fields = append(fields,
			logx.Field(fieldWorkflowNode, meta.WorkflowNode),
			logx.Field(fieldNode, meta.WorkflowNode),
		)
	}
	if meta.ShardTotal > 0 {
		shard := strconv.Itoa(meta.ShardIndex) + "/" + strconv.Itoa(meta.ShardTotal)
		fields = append(fields,
			logx.Field(fieldShard, shard),
			logx.Field(fieldShardIndex, meta.ShardIndex),
			logx.Field(fieldShardTotal, meta.ShardTotal),
		)
	}
	return fields
}

// TraceAttributesFromMeta 把请求元数据映射成统一的 trace attributes。
func TraceAttributesFromMeta(meta *requestctx.Meta) []attribute.KeyValue {
	if meta == nil {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, 28)
	if meta.TraceID != "" {
		attrs = append(attrs, attribute.String("app."+fieldTraceID, meta.TraceID))
	}
	if meta.SpanID != "" {
		attrs = append(attrs, attribute.String("app."+fieldSpanID, meta.SpanID))
	}
	route := strings.TrimSpace(meta.Route)
	if route == "" {
		route = strings.TrimSpace(meta.Path)
	}
	if route != "" {
		attrs = append(attrs, attribute.String("http.route", route), attribute.String("app."+fieldRoute, route))
	}
	if meta.Method != "" {
		attrs = append(attrs, attribute.String("http.method", meta.Method), attribute.String("app."+fieldHTTPMethod, meta.Method))
	}
	if meta.Path != "" {
		attrs = append(attrs, attribute.String("url.path", meta.Path), attribute.String("app."+fieldPath, meta.Path))
	}
	if meta.Locale != "" {
		attrs = append(attrs, attribute.String("app."+fieldLocale, meta.Locale))
	}
	if meta.ClientIP != "" {
		attrs = append(attrs, attribute.String("client.address", meta.ClientIP), attribute.String("app.client_ip", meta.ClientIP))
	}
	if meta.UserID > 0 {
		attrs = append(attrs,
			attribute.String("enduser.id", strconv.FormatInt(meta.UserID, 10)),
			attribute.Int64("app."+fieldUID, meta.UserID),
			attribute.Int64("app."+fieldUserID, meta.UserID),
		)
	}
	if meta.UserName != "" {
		attrs = append(attrs, attribute.String("enduser.name", meta.UserName), attribute.String("app."+fieldUserName, meta.UserName))
	}
	if meta.Node != "" {
		attrs = append(attrs, attribute.String("app."+fieldNode, meta.Node))
	}
	if meta.Mode != "" {
		attrs = append(attrs, attribute.String("app."+fieldMode, meta.Mode))
	}
	if meta.HTTPStatus > 0 {
		attrs = append(attrs, attribute.Int("http.status_code", meta.HTTPStatus), attribute.Int("app."+fieldHTTPStatus, meta.HTTPStatus))
	}
	if meta.BizCode > 0 {
		attrs = append(attrs, attribute.Int("app."+fieldBizCode, meta.BizCode))
	}
	if meta.BizMessage != "" {
		attrs = append(attrs, attribute.String("app."+fieldBizMessage, meta.BizMessage))
	}
	if meta.ErrorMessage != "" {
		attrs = append(attrs, attribute.String("app."+fieldErrorMsg, meta.ErrorMessage))
	}
	if meta.LatencyMS > 0 {
		attrs = append(attrs, attribute.Int64("app.latency_ms", meta.LatencyMS))
	}
	if meta.TaskID != "" {
		attrs = append(attrs, attribute.String("app."+fieldTaskID, meta.TaskID))
	}
	if meta.WorkflowID != "" {
		attrs = append(attrs, attribute.String("app."+fieldWorkflowID, meta.WorkflowID))
	}
	if meta.WorkflowNode != "" {
		attrs = append(attrs, attribute.String("app."+fieldWorkflowNode, meta.WorkflowNode), attribute.String("app."+fieldNode, meta.WorkflowNode))
	}
	if meta.ShardTotal > 0 {
		shard := strconv.Itoa(meta.ShardIndex) + "/" + strconv.Itoa(meta.ShardTotal)
		attrs = append(attrs,
			attribute.String("app."+fieldShard, shard),
			attribute.Int("app."+fieldShardIndex, meta.ShardIndex),
			attribute.Int("app."+fieldShardTotal, meta.ShardTotal),
		)
	}
	return attrs
}

// BindContext 将当前请求字段绑定进 logx context。
func BindContext(ctx context.Context) context.Context {
	fields := publicLogFields(FieldsFromContext(ctx))
	if len(fields) == 0 {
		return ctx
	}
	return logx.ContextWithFields(ctx, fields...)
}

// LoggerWithCallerSkip 返回带 caller skip 的底层 logger，供第三方 logger 适配器使用。
func LoggerWithCallerSkip(skip int) logx.Logger {
	return logx.WithCallerSkip(positiveSkip(skip))
}
