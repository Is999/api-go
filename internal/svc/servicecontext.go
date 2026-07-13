package svc

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"api/internal/config"
	"api/internal/infra/collectorx"

	utils "github.com/Is999/go-utils"
	tablecache "github.com/Is999/table-cache"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SiteDatabases 保存主库和可选命名扩展库连接。
type SiteDatabases struct {
	MainDB   *gorm.DB            // 默认主库连接
	NamedDBs map[DBName]*gorm.DB // 可选扩展库连接
}

// Dependencies 表示 ServiceContext 运行所需的外部依赖集合。
type Dependencies struct {
	SiteDBs           SiteDatabases         // 主库与可选扩展库连接集合
	Rds               redis.UniversalClient // Redis 客户端
	SnowflakeLease    SnowflakeLease        // 雪花 node_id Redis 租约
	TableCacheMetrics tablecache.Metrics    // 表缓存运行指标记录器
}

// SnowflakeLease 约束雪花 node_id 租约的关闭能力。
type SnowflakeLease interface {
	Ready(context.Context) error
	Close(context.Context) error
}

// ConfigReloadExecutor 约束配置重载执行能力，避免 logic 层直接依赖 bootstrap 实现。
type ConfigReloadExecutor interface {
	ReloadConfig(ctx context.Context, source string) error
}

// Collector 约束 API 业务层需要的 Collector 投递能力。
type Collector interface {
	Enqueue(context.Context, collectorx.Event) (string, error) // 投递一条结构化 Collector 事件
	SetAlertHook(collectorx.AlertHook)                         // 接入运行异常告警钩子
	Ready(context.Context) error                               // 检查 Kafka 投递链路
	Close(context.Context) error                               // 释放 Collector 持有的外部资源
}

// HotReloadStatus 描述 config.yaml 热加载的当前运行状态。
type HotReloadStatus struct {
	Enabled                bool      // 是否启用热加载
	Watching               bool      // 当前是否已启动后台监听
	ConfigFile             string    // 当前监听的配置文件路径
	CheckIntervalSeconds   int       // 当前轮询间隔，单位秒
	ConfigVersion          string    // 当前生效配置版本指纹
	ConfigSummary          string    // 当前配置摘要
	RestartRequired        bool      // 本次热加载后是否需要重启才能完全生效
	RestartReason          string    // 需要重启的原因摘要
	LastStatus             string    // 最近一次处理结果：idle/success/failed
	LastMessage            string    // 最近一次处理结果说明
	LastMessageKey         string    // 最近一次前端展示文案的多语言 key
	LastTriggerSource      string    // 最近一次触发来源
	LastFailureCategory    string    // 最近一次失败分类
	LastCheckedAt          time.Time // 最近一次检查配置文件时间
	LastReloadAt           time.Time // 最近一次触发配置重载时间
	LastSuccessAt          time.Time // 最近一次成功加载时间
	LastFailureAt          time.Time // 最近一次失败时间
	ReloadCount            int64     // 累计成功加载次数
	SuppressedFailureCount int64     // 限频压制的重复失败日志次数
}

// ServiceContext 将外部依赖集中管理。
type ServiceContext struct {
	configValue       atomic.Value          // 当前生效的配置快照
	version           atomic.Value          // 当前配置版本指纹
	reloadValue       atomic.Value          // 配置热加载状态快照
	SiteDBs           SiteDatabases         // 主库与可选扩展库连接集合
	Rds               redis.UniversalClient // Redis 客户端
	SnowflakeLease    SnowflakeLease        // 雪花 node_id Redis 租约
	TableCacheMetrics tablecache.Metrics    // 表缓存运行指标记录器
	TrustedProxies    *utils.TrustedProxies // 启动期解析完成的可信反向代理白名单
	ConfigReload      ConfigReloadExecutor  // 配置热加载执行器
	Collector         Collector             // 通用收集器
	components        *ComponentRegistry    // 启动期组件生命周期清单
}

// NewServiceContext 只接收已经初始化完成的依赖。
func NewServiceContext(c config.Config, version string, deps Dependencies) *ServiceContext {
	trustedProxies, _ := utils.NewTrustedProxies(c.TrustedProxies...)
	if len(c.TrustedProxies) == 0 {
		trustedProxies = nil
	}
	svcCtx := &ServiceContext{
		SiteDBs:           deps.SiteDBs,
		Rds:               deps.Rds,
		SnowflakeLease:    deps.SnowflakeLease,
		TableCacheMetrics: deps.TableCacheMetrics,
		TrustedProxies:    trustedProxies,
	}
	svcCtx.UpdateConfig(c)
	svcCtx.UpdateVersion(version)
	svcCtx.UpdateHotReloadStatus(HotReloadStatus{LastStatus: "idle"})
	return svcCtx
}

// ScopedWithContext 基于当前 ServiceContext 构造一份绑定请求上下文的只读作用域副本。
func (s *ServiceContext) ScopedWithContext(ctx context.Context) *ServiceContext {
	if s == nil {
		return nil
	}
	scoped := &ServiceContext{
		SiteDBs:           s.SiteDBs.WithContext(ctx),
		Rds:               s.Rds,
		SnowflakeLease:    s.SnowflakeLease,
		TableCacheMetrics: s.TableCacheMetrics,
		TrustedProxies:    s.TrustedProxies,
	}
	scoped.configValue.Store(s.CurrentConfig())
	scoped.version.Store(s.CurrentVersion())
	scoped.ConfigReload = s.ConfigReload
	scoped.Collector = s.Collector
	scoped.components = s.components
	scoped.UpdateHotReloadStatus(s.CurrentHotReloadStatus())
	return scoped
}

// ClientIP 仅在远端命中显式可信代理时解析转发头；空配置保留只信任回环代理的安全默认值。
func (s *ServiceContext) ClientIP(r *http.Request) string {
	if s != nil && s.TrustedProxies != nil {
		return utils.ClientIPWithTrustedProxies(r, s.TrustedProxies)
	}
	return utils.ClientIP(r)
}

// ComponentRegistry 返回启动期组件生命周期清单。
func (s *ServiceContext) ComponentRegistry() *ComponentRegistry {
	if s == nil {
		return nil
	}
	return s.components
}

// SetComponentRegistry 设置启动期组件生命周期清单。
func (s *ServiceContext) SetComponentRegistry(registry *ComponentRegistry) {
	if s == nil {
		return
	}
	s.components = registry
}

// CurrentConfig 返回当前生效的配置快照。
func (s *ServiceContext) CurrentConfig() config.Config {
	if s == nil {
		return config.Config{}
	}
	if cfg, ok := s.configValue.Load().(config.Config); ok {
		return cfg
	}
	return config.Config{}
}

// UpdateConfig 原子替换运行期配置快照。
func (s *ServiceContext) UpdateConfig(c config.Config) {
	if s == nil {
		return
	}
	s.configValue.Store(c)
}

// CurrentVersion 返回当前配置版本指纹。
func (s *ServiceContext) CurrentVersion() string {
	if s == nil {
		return ""
	}
	if version, ok := s.version.Load().(string); ok {
		return version
	}
	return ""
}

// UpdateVersion 原子替换配置版本指纹。
func (s *ServiceContext) UpdateVersion(version string) {
	if s == nil {
		return
	}
	s.version.Store(version)
}

// CurrentHotReloadStatus 返回当前热加载状态快照。
func (s *ServiceContext) CurrentHotReloadStatus() HotReloadStatus {
	if s == nil {
		return HotReloadStatus{}
	}
	if status, ok := s.reloadValue.Load().(HotReloadStatus); ok {
		return status
	}
	return HotReloadStatus{}
}

// UpdateHotReloadStatus 原子替换热加载状态快照。
func (s *ServiceContext) UpdateHotReloadStatus(status HotReloadStatus) {
	if s == nil {
		return
	}
	s.reloadValue.Store(status)
}

// Lookup 根据数据库名称返回连接，空名称和 main 都指向主库。
func (s SiteDatabases) Lookup(database DBName) *gorm.DB {
	name := NormalizeDBName(database)
	if name == DatabaseMain {
		return s.MainDB
	}
	if s.NamedDBs == nil {
		return nil
	}
	return s.NamedDBs[name]
}

// WithContext 为所有站点库连接绑定请求上下文。
func (s SiteDatabases) WithContext(ctx context.Context) SiteDatabases {
	s.MainDB = withDBContext(s.MainDB, ctx)
	if len(s.NamedDBs) > 0 {
		namedDBs := make(map[DBName]*gorm.DB, len(s.NamedDBs))
		for name, db := range s.NamedDBs {
			namedDBs[name] = withDBContext(db, ctx)
		}
		s.NamedDBs = namedDBs
	}
	return s
}

// withDBContext 为数据库会话绑定请求上下文，空连接保持 nil。
func withDBContext(db *gorm.DB, ctx context.Context) *gorm.DB {
	if db == nil {
		return nil
	}
	return db.WithContext(ctx)
}
