package auth

import corelogic "api/internal/logic"

const (
	// RuntimeRegistryAuthSecurityEvent 表示认证风控事件投递入口。
	RuntimeRegistryAuthSecurityEvent = "auth_security_event"
)

// runtimeRegistrySpecs 是认证业务运行时注册入口的清单源。
var runtimeRegistrySpecs = []corelogic.RuntimeRegistrySpec{
	{
		Name:        RuntimeRegistryAuthSecurityEvent,
		File:        "internal/logic/auth/auth_event.go",
		Method:      "auth.AuthCollectorBizType / RecordAuthEvent",
		Description: "投递脱敏认证风控事件到轻量 Collector",
	},
}

// RuntimeRegistrySpecs 返回认证业务运行时注册入口规格快照。
func RuntimeRegistrySpecs() []corelogic.RuntimeRegistrySpec {
	return append([]corelogic.RuntimeRegistrySpec(nil), runtimeRegistrySpecs...)
}
