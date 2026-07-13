package appalert

import (
	"strconv"
	"strings"

	"api/internal/bootstrap/hotreload"
	"api/internal/infra/collectorx"
	"api/internal/infra/larkx"
)

// LifecycleFailure 生成 API 生命周期失败告警。
func LifecycleFailure(phase, hookName string, err error) larkx.RuntimeAlert {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		phase = "unknown"
	}
	hookName = strings.TrimSpace(hookName)
	status := "API 生命周期操作失败，进程启动或平滑停止未按预期完成"
	if phase == "start" {
		status = "API 启动失败，服务未进入可用状态"
	} else if phase == "stop" {
		status = "API 平滑停止失败，部分资源可能未完整释放"
	}
	return larkx.RuntimeAlert{
		Kind:      "app_lifecycle_failed",
		Title:     "【P1 API 生命周期失败】",
		Status:    status,
		Component: "app_lifecycle",
		Operation: phase,
		BizType:   hookName,
		UniqueKey: phase + ":" + hookName,
		Reason:    errorReason(err),
		Advice:    "请检查 API 启动日志、配置、数据库、Redis 与 tracing 等外部依赖；停止阶段失败时需确认连接池和后台组件是否残留。",
	}
}

// ConfigReloadFailure 生成 config.yaml 热加载失败告警。
func ConfigReloadFailure(message string, err error, source, category, configFile string) larkx.RuntimeAlert {
	return larkx.RuntimeAlert{
		Kind:      "config_reload_failed",
		Title:     "【P1 API 配置热加载失败】",
		Status:    "配置热加载未生效，当前进程继续使用上一版有效配置",
		Component: "config_reload",
		Operation: hotreload.FailureCategory(category),
		BizType:   hotreload.Source(source),
		UniqueKey: hotreload.Source(source) + ":" + hotreload.FailureCategory(category),
		Reason:    configReloadReason(message, err, configFile),
		Advice:    "请检查配置文件内容、外部 include 文件和启动期配置边界；修复后重新触发热加载，并关注 restartRequired/restartReason。",
	}
}

// CollectorRuntimeAlert 将 Collector 内部异常转换为 API 统一运行异常。
func CollectorRuntimeAlert(alert collectorx.RuntimeAlert) larkx.RuntimeAlert {
	reason := strings.TrimSpace(alert.Reason)
	if alert.Count > 0 {
		countText := "影响数量=" + strconv.Itoa(alert.Count)
		if reason != "" {
			reason += "；" + countText
		} else {
			reason = countText
		}
	}
	return larkx.RuntimeAlert{
		Kind:       strings.TrimSpace(alert.Kind),
		Title:      strings.TrimSpace(alert.Title),
		Status:     strings.TrimSpace(alert.Status),
		Component:  strings.TrimSpace(alert.Component),
		Operation:  strings.TrimSpace(alert.Operation),
		BizType:    strings.TrimSpace(alert.BizType),
		Transport:  strings.TrimSpace(alert.Channel),
		UniqueKey:  strings.TrimSpace(alert.UniqueKey),
		Reason:     reason,
		Advice:     strings.TrimSpace(alert.Advice),
		OccurredAt: alert.OccurredAt,
	}
}

// configReloadReason 拼接配置热加载失败原因，保留配置文件路径便于排查。
func configReloadReason(message string, err error, configFile string) string {
	parts := make([]string, 0, 3)
	if message = strings.TrimSpace(message); message != "" {
		parts = append(parts, message)
	}
	if configFile = strings.TrimSpace(configFile); configFile != "" {
		parts = append(parts, "配置文件="+configFile)
	}
	if reason := errorReason(err); reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "；")
}

// errorReason 返回告警原因文本，避免空错误导致告警字段缺失。
func errorReason(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
