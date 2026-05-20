package logic

// RuntimeRegistrySpec 描述通用业务逻辑提供的轻量运行时扩展入口。
type RuntimeRegistrySpec struct {
	Name        string // 注册名称，必须在运行时扩展清单中唯一
	File        string // 注册实现所在文件
	Method      string // 注册入口方法
	Description string // 注册项中文说明
}

const (
	// RuntimeRegistryCacheRebuildLock 表示缓存重建分布式锁入口。
	RuntimeRegistryCacheRebuildLock = "cache_rebuild_lock"
)

// runtimeRegistrySpecs 是通用业务逻辑运行时注册入口的清单源。
var runtimeRegistrySpecs = []RuntimeRegistrySpec{
	{
		Name:        RuntimeRegistryCacheRebuildLock,
		File:        "internal/infra/redsync/lock.go + internal/logic/cache_guard.go",
		Method:      "RebuildCacheWithLock / TryRebuildCacheWithLock",
		Description: "使用 redsync 保护缓存重建",
	},
}

// RuntimeRegistrySpecs 返回通用业务逻辑运行时注册入口规格快照。
func RuntimeRegistrySpecs() []RuntimeRegistrySpec {
	return append([]RuntimeRegistrySpec(nil), runtimeRegistrySpecs...)
}
