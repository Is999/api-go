package auth

// RuntimeRegistrySpec 描述认证业务提供的轻量运行时扩展入口。
type RuntimeRegistrySpec struct {
	Name        string // 注册名称，必须在运行时扩展清单中唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法
	Description string // 注册项中文说明
}

const (
	// RuntimeRegistryAuthSecurityEvent 表示认证风控事件投递入口。
	RuntimeRegistryAuthSecurityEvent = "auth_security_event"
)

// runtimeRegistrySpecs 是认证业务运行时注册入口的清单源。
var runtimeRegistrySpecs = []RuntimeRegistrySpec{
	{
		Name:        RuntimeRegistryAuthSecurityEvent,
		File:        "internal/logic/auth/auth_event.go",
		Method:      "auth.AuthCollectorBizType / RecordAuthEvent",
		Description: "投递脱敏认证风控事件到轻量 Collector",
	},
}

// RuntimeRegistrySpecs 返回认证业务运行时注册入口规格快照。
func RuntimeRegistrySpecs() []RuntimeRegistrySpec {
	return append([]RuntimeRegistrySpec(nil), runtimeRegistrySpecs...)
}
