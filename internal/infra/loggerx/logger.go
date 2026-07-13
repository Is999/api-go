package loggerx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
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
	fieldUID          = "uid"           // 用户 ID 短字段名
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
	fieldLatencyMS    = "latency_ms"    // 耗时毫秒数字段名
	fieldSuccess      = "success"       // 成功状态字段名
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

// publicLogFieldNames 仅保留跨请求、任务和错误排障共用的顶层检索字段。
// 路径、SQL、payload 和统计量等详情写入 content，避免日志平台字段膨胀。
var publicLogFieldNames = map[string]struct{}{
	fieldTraceID:     {},
	fieldSpanID:      {},
	fieldRoute:       {},
	fieldHTTPMethod:  {},
	fieldIP:          {},
	fieldUserID:      {},
	fieldHTTPStatus:  {},
	fieldBizCode:     {},
	fieldError:       {},
	fieldErrorChain:  {},
	fieldErrorCaller: {},
	fieldCaller:      {},
	fieldLogCaller:   {},
	fieldTaskID:      {},
	fieldWorkflowID:  {},
	fieldMode:        {},
	fieldNode:        {},
	fieldShard:       {},
	fieldShardIndex:  {},
	fieldShardTotal:  {},
	fieldLatencyMS:   {},
	fieldSuccess:     {},
}

// Errorw 统一输出带错误链路的错误日志。
func Errorw(ctx context.Context, msg string, err error, fields ...logx.LogField) {
	errorw(ctx, 0, msg, err, fields...)
}

// errorw 写入错误日志，并按调用方传入的额外 skip 修正 caller。
func errorw(ctx context.Context, skip int, msg string, err error, fields ...logx.LogField) {
	fields = appendLogFields(fields, ErrorFields(err)...)
	fields = appendErrorCallerFields(fields, err, callerLocation(runtimeCallerSkip(skip)))
	msg, fields = splitLogFields(msg, appendContextFields(ctx, fields))
	LoggerWithCallerSkip(normalizeCallerSkip(skip)).Errorw(msg, fields...)
}

// ErrorTextw 统一输出只有错误文本的错误日志。
func ErrorTextw(ctx context.Context, msg string, errorText string, fields ...logx.LogField) {
	errorTextw(ctx, 0, msg, errorText, fields...)
}

// errorTextw 写入文本错误日志，适用于尚未构造成 error 的失败原因。
func errorTextw(ctx context.Context, skip int, msg string, errorText string, fields ...logx.LogField) {
	fields = appendLogFields(fields, ErrorTextFields(errorText)...)
	fields = appendCallerField(fields, callerLocation(runtimeCallerSkip(skip)))
	msg, fields = splitLogFields(msg, appendContextFields(ctx, fields))
	LoggerWithCallerSkip(normalizeCallerSkip(skip)).Errorw(msg, fields...)
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
	msg, fields = splitLogFields(msg, appendContextFields(ctx, fields))
	LoggerWithCallerSkip(normalizeCallerSkip(skip)).Infow(msg, fields...)
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
	msg, fields = splitLogFields(msg, appendContextFields(ctx, fields))
	LoggerWithCallerSkip(normalizeCallerSkip(skip)).Debugw(msg, fields...)
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
	msg, fields = splitLogFields(msg, appendContextFields(ctx, fields))
	LoggerWithCallerSkip(normalizeCallerSkip(skip)).Sloww(msg, fields...)
}

// appendContextFields 先补上下文字段，再追加调用点字段，保证调用点的事件结果优先。
func appendContextFields(ctx context.Context, fields []logx.LogField) []logx.LogField {
	return appendLogFields(FieldsFromContext(ctx), fields...)
}

// splitLogFields 将非公共字段折叠进 content，减少日志平台动态字段数量。
func splitLogFields(msg string, fields []logx.LogField) (string, []logx.LogField) {
	if len(fields) == 0 {
		return msg, nil
	}
	publicFields := make([]logx.LogField, 0, len(fields))
	details := make([]string, 0, len(fields))
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		if field.Key == "" {
			field.Key = "field"
		}
		if isPublicLogField(field.Key) {
			publicFields = append(publicFields, field)
			continue
		}
		details = append(details, formatLogDetail(field))
	}
	if len(details) == 0 {
		return msg, publicFields
	}
	msg = strings.TrimSpace(msg)
	detailText := strings.Join(details, " ")
	if msg == "" {
		return detailText, publicFields
	}
	return msg + " | " + detailText, publicFields
}

// publicLogFields 过滤出允许写入顶层索引的公共字段，供直接绑定 logx context 的场景复用。
func publicLogFields(fields []logx.LogField) []logx.LogField {
	if len(fields) == 0 {
		return nil
	}
	publicFields := make([]logx.LogField, 0, len(fields))
	for _, field := range fields {
		field.Key = strings.TrimSpace(field.Key)
		if field.Key == "" || !isPublicLogField(field.Key) {
			continue
		}
		publicFields = append(publicFields, field)
	}
	return publicFields
}

// isPublicLogField 判断字段是否属于公共检索字段。
func isPublicLogField(key string) bool {
	_, ok := publicLogFieldNames[key]
	return ok
}

// formatLogDetail 把单个非公共字段格式化成稳定的 key=value 文本。
func formatLogDetail(field logx.LogField) string {
	return field.Key + "=" + formatLogValue(field.Value)
}

// formatLogValue 将字段值转换成单行文本，复杂值优先使用 JSON 保真。
func formatLogValue(value any) string {
	switch val := value.(type) {
	case nil:
		return "null"
	case string:
		return formatLogString(val)
	case error:
		return formatLogString(val.Error())
	case fmt.Stringer:
		return formatLogString(val.String())
	default:
		raw, err := json.Marshal(val)
		if err == nil {
			return string(raw)
		}
		return formatLogString(fmt.Sprint(val))
	}
}

// formatLogString 将字符串压成单行，必要时加 JSON 引号避免空格和等号歧义。
func formatLogString(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value))
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " =\"'{}[],:|") {
		raw, err := json.Marshal(value)
		if err == nil {
			return string(raw)
		}
	}
	return value
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
