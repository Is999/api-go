package loggerx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"api/internal/config"
	"api/internal/requestctx"

	"github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
	jsoniter "github.com/json-iterator/go"
	"github.com/zeromicro/go-zero/core/logx"
	"go.opentelemetry.io/otel/attribute"
)

// 统一日志字段名，保持日志、trace 和排障维度一致。
const (
	// loggerxCallerSkip 表示从 loggerx 内部写入函数跳到真实业务调用点需要额外跳过的栈层数。
	loggerxCallerSkip = 2
	// loggerxRuntimeCallerSkip 表示 runtime.Caller 从内部写入函数跳到业务调用点的默认栈层数。
	loggerxRuntimeCallerSkip = 3
	// goUtilsCallerSkip 表示 go-utils 日志适配器自身增加的一层封装。
	goUtilsCallerSkip = 1

	fieldTraceID      = "trace_id"      // trace id 日志字段名
	fieldSpanID       = "span_id"       // span id 日志字段名
	fieldRoute        = "route"         // 稳定路由别名字段名
	fieldHTTPMethod   = "http_method"   // HTTP 方法字段名
	fieldPath         = "path"          // HTTP 路径字段名
	fieldLocale       = "locale"        // 请求语言字段名
	fieldIP           = "ip"            // 客户端 IP 字段名
	fieldUID          = "uid"           // 兼容 uid 字段名
	fieldUserID       = "user_id"       // 用户 ID 字段名
	fieldUserName     = "user_name"     // 用户名字段名
	fieldNode         = "node"          // 服务节点或工作流节点字段名
	fieldMode         = "mode"          // 运行模式字段名
	fieldHTTPStatus   = "http_status"   // HTTP 状态码字段名
	fieldBizCode      = "biz_code"      // 业务码字段名
	fieldBizMessage   = "biz_message"   // 业务响应文案字段名
	fieldError        = "error"         // 错误摘要字段名
	fieldErrorChain   = "error_chain"   // 错误链字段名
	fieldErrorTrace   = "error_trace"   // 错误追踪文本字段名
	fieldErrorCaller  = "error_caller"  // 错误产生位置字段名
	fieldCaller       = "caller"        // 业务定位 caller 字段名
	fieldLogCaller    = "log_caller"    // 日志打印 caller 字段名
	fieldErrorMsg     = "error_message" // 错误消息字段名
	fieldTaskID       = "task_id"       // 异步任务 ID 字段名
	fieldWorkflowID   = "workflow_id"   // 工作流 ID 字段名
	fieldWorkflowNode = "workflow_node" // 工作流节点字段名
	fieldShard        = "shard"         // 分片摘要字段名
	fieldShardIndex   = "shard_index"   // 分片索引字段名
	fieldShardTotal   = "shard_total"   // 分片总数字段名
)

// 带单位的通用日志字段名。
const (
	// FieldIntervalSeconds 表示轮询、调度或重试间隔秒数。
	FieldIntervalSeconds = "interval_seconds"
	// FieldWindowStartUnix 表示时间窗口起点 Unix 秒。
	FieldWindowStartUnix = "window_start_unix"
	// FieldWindowEndUnix 表示时间窗口终点排他边界 Unix 秒。
	FieldWindowEndUnix = "window_end_unix"
)

// Setup 初始化 go-zero 日志，并在文件输出模式下额外镜像到 stdout 方便容器采集。
func Setup(c config.Config) {
	if shouldMoveBuiltinCaller(c.Log.FieldKeys.CallerKey) {
		c.Log.FieldKeys.CallerKey = fieldLogCaller
	}
	logx.MustSetup(c.Log)
	if strings.EqualFold(c.Log.Mode, "file") {
		logx.AddWriter(logx.NewWriter(os.Stdout))
	}
	errors.SetStackDepth(32)
	errors.SetTraceEnabled(true)
	utils.Configure(
		utils.WithJSON(jsoniter.Marshal, jsoniter.Unmarshal),
		utils.WithLogger(newGoUtilsLogger(nil)),
	)
}

// goUtilsLogger 把 github.com/Is999/go-utils 的结构化日志接口适配到 go-zero logx。
type goUtilsLogger struct {
	fields []any // fields 保存 With 传入的 slog 风格键值对
}

// newGoUtilsLogger 创建 go-utils 日志适配器。
func newGoUtilsLogger(fields []any) *goUtilsLogger {
	copied := make([]any, 0, len(fields))
	copied = append(copied, fields...)
	return &goUtilsLogger{fields: copied}
}

// Debug 输出调试日志。
func (l *goUtilsLogger) Debug(msg string, args ...any) {
	DebugwSkip(nil, goUtilsCallerSkip, msg, l.logFields(args...)...)
}

// Info 输出信息日志。
func (l *goUtilsLogger) Info(msg string, args ...any) {
	InfowSkip(nil, goUtilsCallerSkip, msg, l.logFields(args...)...)
}

// Warn 输出警告日志。
func (l *goUtilsLogger) Warn(msg string, args ...any) {
	SlowwSkip(nil, goUtilsCallerSkip, msg, l.logFields(args...)...)
}

// Error 输出错误日志。
func (l *goUtilsLogger) Error(msg string, args ...any) {
	ErrorTextwSkip(nil, goUtilsCallerSkip, msg, msg, l.logFields(args...)...)
}

// With 创建携带固定字段的新日志对象。
func (l *goUtilsLogger) With(args ...any) utils.Logger {
	fields := make([]any, 0, len(l.fields)+len(args))
	fields = append(fields, l.fields...)
	fields = append(fields, args...)
	return newGoUtilsLogger(fields)
}

// Enabled 返回日志级别是否启用。
func (l *goUtilsLogger) Enabled(_ context.Context, _ utils.LogLevel) bool {
	return true
}

// logFields 把 slog 风格键值参数转换成 logx 字段。
func (l *goUtilsLogger) logFields(args ...any) []logx.LogField {
	merged := make([]any, 0, len(l.fields)+len(args))
	merged = append(merged, l.fields...)
	merged = append(merged, args...)
	fields := make([]logx.LogField, 0, (len(merged)+1)/2)
	for i := 0; i < len(merged); i += 2 {
		key, ok := merged[i].(string)
		if !ok || strings.TrimSpace(key) == "" {
			key = "field"
		}
		var value any = ""
		if i+1 < len(merged) {
			value = merged[i+1]
		}
		fields = append(fields, logx.Field(key, value))
	}
	return fields
}

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

// ErrorChain 把错误链渲染为单行 JSON 字符串。
func ErrorChain(err error) string {
	if err == nil {
		return ""
	}
	traceJSON := strings.TrimSpace(errors.TraceJSON(err))
	if traceJSON != "" {
		return traceJSON
	}
	summary := strings.TrimSpace(err.Error())
	return summary
}

// ErrorTrace 把错误链渲染为便于人眼检索的单行文本。
func ErrorTrace(err error) string {
	if err == nil {
		return ""
	}
	trace := strings.TrimSpace(errors.TraceString(err))
	if trace != "" {
		return trace
	}
	return ErrorChain(err)
}

// ErrorCaller 返回错误链中最早的业务栈帧。
func ErrorCaller(err error) string {
	if err == nil {
		return ""
	}
	if caller := firstTraceCallerFromJSON(ErrorChain(err)); caller != "" {
		return caller
	}
	return firstTraceCallerFromText(ErrorTrace(err))
}

// ErrorFields 返回统一错误日志字段。
func ErrorFields(err error) []logx.LogField {
	if err == nil {
		return nil
	}
	summary := strings.TrimSpace(err.Error())
	chain := ErrorChain(err)
	fields := []logx.LogField{
		logx.Field(fieldError, summary),
		logx.Field(fieldErrorChain, chain),
	}
	if trace := ErrorTrace(err); trace != "" && trace != summary {
		fields = append(fields, logx.Field(fieldErrorTrace, trace))
	}
	if caller := ErrorCaller(err); caller != "" {
		fields = append(fields, logx.Field(fieldErrorCaller, caller))
	}
	return fields
}

// ErrorTextFields 返回纯文本错误对应的统一日志字段。
func ErrorTextFields(message string) []logx.LogField {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	return []logx.LogField{
		logx.Field(fieldError, message),
		logx.Field(fieldErrorChain, message),
	}
}

// Errorw 统一输出带错误链路的错误日志。
func Errorw(ctx context.Context, msg string, err error, fields ...logx.LogField) {
	errorw(ctx, 0, msg, err, fields...)
}

// errorw 写入错误日志，并按调用方传入的额外 skip 修正 caller。
func errorw(ctx context.Context, skip int, msg string, err error, fields ...logx.LogField) {
	fields = appendLogFields(fields, ErrorFields(err)...)
	fields = appendErrorCallerFields(fields, err, callerLocation(runtimeCallerSkip(skip)))
	loggerFor(ctx, logxCallerSkip(skip)).Errorw(msg, fields...)
}

// ErrorTextw 统一输出只有错误文本的错误日志。
func ErrorTextw(ctx context.Context, msg string, errorText string, fields ...logx.LogField) {
	errorTextw(ctx, 0, msg, errorText, fields...)
}

// errorTextw 写入文本错误日志，适用于尚未构造成 error 的失败原因。
func errorTextw(ctx context.Context, skip int, msg string, errorText string, fields ...logx.LogField) {
	fields = appendLogFields(fields, ErrorTextFields(errorText)...)
	fields = appendCallerField(fields, callerLocation(runtimeCallerSkip(skip)))
	loggerFor(ctx, logxCallerSkip(skip)).Errorw(msg, fields...)
}

// ErrorwSkip 统一输出带 caller skip 的错误日志。
func ErrorwSkip(ctx context.Context, skip int, msg string, err error, fields ...logx.LogField) {
	errorw(ctx, skip, msg, err, fields...)
}

// ErrorTextwSkip 统一输出带 caller skip 的文本错误日志。
func ErrorTextwSkip(ctx context.Context, skip int, msg string, errorText string, fields ...logx.LogField) {
	errorTextw(ctx, skip, msg, errorText, fields...)
}

// Infow 统一输出信息日志。
func Infow(ctx context.Context, msg string, fields ...logx.LogField) {
	infow(ctx, 0, msg, fields...)
}

// InfowSkip 输出带 caller skip 的信息日志，适用于第三方适配器等额外封装层。
func InfowSkip(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	infow(ctx, skip, msg, fields...)
}

// infow 写入信息日志，并统一补充业务 caller 字段。
func infow(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	fields = appendCallerField(fields, callerLocation(runtimeCallerSkip(skip)))
	loggerFor(ctx, logxCallerSkip(skip)).Infow(msg, fields...)
}

// Debugw 统一输出调试日志。
func Debugw(ctx context.Context, msg string, fields ...logx.LogField) {
	debugw(ctx, 0, msg, fields...)
}

// DebugwSkip 输出带 caller skip 的调试日志，适用于第三方适配器等额外封装层。
func DebugwSkip(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	debugw(ctx, skip, msg, fields...)
}

// debugw 写入调试日志，并统一补充业务 caller 字段。
func debugw(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	fields = appendCallerField(fields, callerLocation(runtimeCallerSkip(skip)))
	loggerFor(ctx, logxCallerSkip(skip)).Debugw(msg, fields...)
}

// Sloww 统一输出慢操作日志。
func Sloww(ctx context.Context, msg string, fields ...logx.LogField) {
	sloww(ctx, 0, msg, fields...)
}

// SlowwSkip 输出带 caller skip 的慢操作日志，适用于第三方适配器等额外封装层。
func SlowwSkip(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	sloww(ctx, skip, msg, fields...)
}

// sloww 写入慢操作日志，并统一补充业务 caller 字段。
func sloww(ctx context.Context, skip int, msg string, fields ...logx.LogField) {
	fields = appendCallerField(fields, callerLocation(runtimeCallerSkip(skip)))
	loggerFor(ctx, logxCallerSkip(skip)).Sloww(msg, fields...)
}

// appendLogFields 合并日志字段，保持调用方字段在前。
func appendLogFields(base []logx.LogField, extra ...logx.LogField) []logx.LogField {
	merged := make([]logx.LogField, 0, len(base)+len(extra))
	merged = append(merged, base...)
	merged = append(merged, extra...)
	return merged
}

// runtimeCallerSkip 返回 runtime.Caller 使用的最终 skip。
func runtimeCallerSkip(skip int) int {
	return loggerxRuntimeCallerSkip + positiveSkip(skip)
}

// logxCallerSkip 返回 go-zero logx 使用的最终 skip。
func logxCallerSkip(skip int) int {
	return normalizeCallerSkip(skip)
}

// appendErrorCallerFields 优先使用 error 产生位置作为业务 caller。
func appendErrorCallerFields(fields []logx.LogField, err error, logCaller string) []logx.LogField {
	sourceCaller := ErrorCaller(err)
	if sourceCaller == "" {
		sourceCaller = logCaller
	}
	fields = appendCallerField(fields, sourceCaller)
	if logCaller != "" && logCaller != sourceCaller {
		fields = appendLogFields(fields, logx.Field(fieldLogCaller, logCaller))
	}
	return fields
}

// appendCallerField 追加统一 caller 字段，空值不输出。
func appendCallerField(fields []logx.LogField, caller string) []logx.LogField {
	caller = strings.TrimSpace(caller)
	if caller == "" {
		return fields
	}
	return appendLogFields(fields, logx.Field(fieldCaller, caller))
}

// errorTraceNode 表示 go-utils errors.TraceJSON 中的错误链节点。
type errorTraceNode struct {
	Trace []string          `json:"trace"` // Trace 保存当前错误节点的栈帧文本。
	Err   json.RawMessage   `json:"err"`   // Err 保存单个下级错误节点。
	Errs  []json.RawMessage `json:"errs"`  // Errs 保存多个下级错误节点。
}

// firstTraceCallerFromJSON 从错误链 JSON 中提取最早的业务栈帧。
func firstTraceCallerFromJSON(traceJSON string) string {
	traceJSON = strings.TrimSpace(traceJSON)
	if traceJSON == "" || traceJSON == "null" {
		return ""
	}
	var node errorTraceNode
	if err := json.Unmarshal([]byte(traceJSON), &node); err != nil {
		return ""
	}
	return firstTraceCallerFromNode(node)
}

// firstTraceCallerFromNode 按当前节点、单错误、多错误顺序查找业务栈帧。
func firstTraceCallerFromNode(node errorTraceNode) string {
	for _, frame := range node.Trace {
		if caller := traceFrameLocation(frame); caller != "" {
			return caller
		}
	}
	if caller := firstTraceCallerFromRaw(node.Err); caller != "" {
		return caller
	}
	for _, raw := range node.Errs {
		if caller := firstTraceCallerFromRaw(raw); caller != "" {
			return caller
		}
	}
	return ""
}

// firstTraceCallerFromRaw 解析原始错误节点并提取业务栈帧。
func firstTraceCallerFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var node errorTraceNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}
	return firstTraceCallerFromNode(node)
}

// firstTraceCallerFromText 从文本错误链中提取业务栈帧。
func firstTraceCallerFromText(traceText string) string {
	traceText = strings.TrimSpace(traceText)
	if traceText == "" {
		return ""
	}
	return traceFrameLocation(traceText)
}

// traceFrameLocation 从单个栈帧文本中截取短文件名和行号。
func traceFrameLocation(frame string) string {
	frame = strings.TrimSpace(frame)
	if frame == "" {
		return ""
	}
	if start := strings.LastIndex(frame, " ("); start >= 0 && strings.HasSuffix(frame, ")") {
		frame = strings.TrimSuffix(frame[start+2:], ")")
	}
	idx := strings.LastIndex(frame, ".go:")
	if idx < 0 {
		return ""
	}
	end := idx + len(".go:")
	for end < len(frame) && frame[end] >= '0' && frame[end] <= '9' {
		end++
	}
	start := strings.LastIndexAny(frame[:idx], " \t(")
	if start >= 0 {
		return frame[start+1 : end]
	}
	return frame[:end]
}

// callerLocation 返回 runtime.Caller 对应的短文件名和行号。
func callerLocation(skip int) string {
	if skip < 0 {
		skip = 0
	}
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	return shortCaller(file, line)
}

// shortCaller 保留最后两段路径，避免日志 caller 过长。
func shortCaller(file string, line int) string {
	file = filepath.ToSlash(file)
	idx := strings.LastIndexByte(file, '/')
	if idx >= 0 {
		if prev := strings.LastIndexByte(file[:idx], '/'); prev >= 0 {
			file = file[prev+1:]
		}
	}
	return file + ":" + strconv.Itoa(line)
}

// shouldMoveBuiltinCaller 判断是否需要把 go-zero 内置 caller 改名为 log_caller。
func shouldMoveBuiltinCaller(callerKey string) bool {
	callerKey = strings.TrimSpace(callerKey)
	return callerKey == "" || callerKey == fieldCaller
}

// positiveSkip 把负数 skip 归零，避免调用点向错误方向偏移。
func positiveSkip(skip int) int {
	if skip < 0 {
		return 0
	}
	return skip
}

// normalizeCallerSkip 归一化 caller skip，避免传入负数导致调用点偏移。
func normalizeCallerSkip(skip int) int {
	return loggerxCallerSkip + positiveSkip(skip)
}

// loggerFor 返回带上下文字段和 caller skip 的 logx logger。
func loggerFor(ctx context.Context, skip int) logx.Logger {
	if ctx == nil {
		return LoggerWithCallerSkip(skip)
	}
	return contextLoggerWithCallerSkip(BindContext(ctx), skip)
}

// BindContext 将当前请求字段绑定进 logx context。
func BindContext(ctx context.Context) context.Context {
	fields := FieldsFromContext(ctx)
	if len(fields) == 0 {
		return ctx
	}
	return logx.ContextWithFields(ctx, fields...)
}

// LoggerWithCallerSkip 返回带 caller skip 的底层 logger，供第三方 logger 适配器使用。
func LoggerWithCallerSkip(skip int) logx.Logger {
	return logx.WithCallerSkip(positiveSkip(skip))
}

// contextLoggerWithCallerSkip 返回同时绑定上下文和 caller skip 的底层 logger。
func contextLoggerWithCallerSkip(ctx context.Context, skip int) logx.Logger {
	return logx.WithContext(ctx).WithCallerSkip(positiveSkip(skip))
}
