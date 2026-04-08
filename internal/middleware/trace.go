package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	i18n "gozero_api/common/i18n"
	"gozero_api/internal/infra/loggerx"
	"gozero_api/internal/requestctx"

	"github.com/Is999/go-utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TraceMiddleware 在 HTTP 请求入口创建或继承链路上下文。
type TraceMiddleware struct {
	tracer trace.Tracer // HTTP 入口 span 使用的 tracer 实例
	node   string       // 当前服务节点名称
}

// NewTraceMiddleware 创建服务端 span 中间件。
func NewTraceMiddleware() *TraceMiddleware {
	return &TraceMiddleware{
		tracer: otel.Tracer("gozero_api/http"),
		node:   resolveNodeName(),
	}
}

// Handle 基于标准 W3C tracecontext 继承链路，并兼容前端 X-Trace-Id。
func (m *TraceMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := requestctx.New(r.Context())
		requestctx.SetRequest(ctx, r.Method, r.URL.Path, utils.ClientIP(r))
		requestctx.SetLocale(ctx, i18n.NormalizeLocale(r.Header.Get("Accept-Language")))
		requestctx.SetNode(ctx, m.node)
		requestctx.SetMode(ctx, "api")
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
		ctx = inheritTraceIDFromHeader(ctx, r)

		ctx, span := m.tracer.Start(ctx, r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		sc := span.SpanContext()
		requestctx.SetTrace(ctx, sc.TraceID().String(), sc.SpanID().String())
		span.SetAttributes(loggerx.TraceAttributesFromMeta(requestctx.FromContext(ctx))...)
		ctx = loggerx.BindContext(ctx)

		w.Header().Set(requestctx.HeaderTraceID, sc.TraceID().String())
		w.Header().Set(requestctx.HeaderSpanID, sc.SpanID().String())
		next(w, r.WithContext(ctx))
		syncSpanWithMeta(span, requestctx.FromContext(ctx))
	}
}

// inheritTraceIDFromHeader 兼容前端传入的 X-Trace-Id。
func inheritTraceIDFromHeader(ctx context.Context, r *http.Request) context.Context {
	if r == nil {
		return ctx
	}
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	if strings.TrimSpace(r.Header.Get(requestctx.HeaderTraceParent)) != "" {
		return ctx
	}
	traceID, ok := parseHeaderTraceID(r.Header.Get(requestctx.HeaderTraceID))
	if !ok {
		return ctx
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     newParentSpanID(),
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	if !parent.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, parent)
}

// parseHeaderTraceID 校验并解析 32 位 trace id。
func parseHeaderTraceID(raw string) (trace.TraceID, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "-", ""))
	if len(normalized) != 32 {
		return trace.TraceID{}, false
	}
	if _, err := hex.DecodeString(normalized); err != nil {
		return trace.TraceID{}, false
	}
	traceID, err := trace.TraceIDFromHex(normalized)
	if err != nil || !traceID.IsValid() {
		return trace.TraceID{}, false
	}
	return traceID, true
}

// newParentSpanID 为兼容 trace id 创建临时父 span id。
func newParentSpanID() trace.SpanID {
	var spanID trace.SpanID
	if _, err := rand.Read(spanID[:]); err != nil || !spanID.IsValid() {
		return trace.SpanID{1}
	}
	return spanID
}

// syncSpanWithMeta 在响应结束后同步 route、状态和业务字段到 span。
func syncSpanWithMeta(span trace.Span, meta *requestctx.Meta) {
	if span == nil || meta == nil {
		return
	}
	route := strings.TrimSpace(meta.Route)
	if route == "" {
		route = strings.TrimSpace(meta.Path)
	}
	if route != "" && strings.TrimSpace(meta.Method) != "" {
		span.SetName(meta.Method + " " + route)
	}
	span.SetAttributes(loggerx.TraceAttributesFromMeta(meta)...)
}

// resolveNodeName 读取当前主机名，失败时返回 unknown。
func resolveNodeName() string {
	if name, err := os.Hostname(); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "unknown"
}
