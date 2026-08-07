package logic

// RuntimeRegistrySpec 描述业务子包暴露给默认清单的轻量运行时扩展入口。
type RuntimeRegistrySpec struct {
	Name        string // 注册名称，必须在运行时扩展清单中唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法
	Description string // 注册项中文说明
}
