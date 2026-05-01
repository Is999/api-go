package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"strings"

	keys "api/common/rediskeys"
	"api/internal/config"
	"api/internal/handler"
	"api/internal/infra/collectorx"
	"api/internal/infra/loggerx"
	mysqlx "api/internal/infra/mysql"
	"api/internal/infra/redisx"
	"api/internal/infra/tracing"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"gorm.io/gorm"
)

// App 聚合服务运行所需的配置、HTTP Server 和关闭钩子。
type App struct {
	Server         *rest.Server                // HTTP 服务实例
	ServiceContext *svc.ServiceContext         // 全局服务上下文
	ConfigFile     string                      // 当前应用对应的配置文件路径
	shutdown       func(context.Context) error // tracing 等基础设施关闭钩子
	hotReload      configHotReloadState        // 配置热加载运行态资源
}

// New 负责把依赖装配与 HTTP 服务注册串起来。
func New(ctx context.Context, c config.Config, version string) (*App, error) {
	if err := ValidateDefaultRegistrationManifest(); err != nil {
		return nil, errors.Wrap(err, "校验默认注册清单失败")
	}

	svcCtx, shutdown, err := BuildServiceContext(ctx, c, version)
	if err != nil {
		return nil, errors.Tag(err)
	}

	restConf := c.RestConf
	// 项目已接入自定义 access log 中间件，关闭 go-zero 默认 HTTP 日志。
	restConf.Middlewares.Log = false
	server, err := rest.NewServer(restConf)
	if err != nil {
		_ = closeServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, errors.Wrapf(err, "创建 HTTP 服务失败 host=%s port=%d", restConf.Host, restConf.Port)
	}
	app := &App{
		Server:         server,
		ServiceContext: svcCtx,
		shutdown:       shutdown,
	}
	svcCtx.ConfigReload = app
	handler.RegisterHandlers(server, svcCtx, defaultRouteModules()...)
	return app, nil
}

// BuildServiceContext 统一完成基础设施初始化。
func BuildServiceContext(ctx context.Context, c config.Config, version string) (*svc.ServiceContext, func(context.Context) error, error) {
	loggerx.Setup(c)
	shutdown, err := tracing.Setup(ctx, c.Observability)
	if err != nil {
		return nil, nil, errors.Tag(err)
	}

	siteDBs, err := buildSiteDatabases(ctx, c)
	if err != nil {
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Tag(err)
	}

	rdb, err := redisx.New(ctx, c.Redis, c.Observability)
	if err != nil {
		_ = closeSiteDatabases(siteDBs)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Tag(err)
	}

	svcCtx := svc.NewServiceContext(c, version, svc.Dependencies{
		SiteDBs: siteDBs,
		Rds:     rdb,
	})
	previousRuntime := publishRuntimeConfig(c)
	rollbackRuntime := func() {
		restoreRuntimeConfig(previousRuntime)
	}
	collectorManager, err := collectorx.New(collectorConfigWithAppID(c), rdb)
	if err != nil {
		rollbackRuntime()
		_ = closeServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Tag(err)
	}
	if err := collectorx.RegisterDefaultProcessors(collectorManager); err != nil {
		rollbackRuntime()
		_ = closeServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Wrapf(err, "注册默认 Collector Processor 失败")
	}
	svcCtx.Collector = collectorManager
	componentRegistry, err := buildDefaultComponentRegistry(svcCtx)
	if err != nil {
		rollbackRuntime()
		_ = closeServiceContextResources(svcCtx)
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		return nil, nil, errors.Wrapf(err, "构建组件生命周期注册表失败")
	}
	svcCtx.SetComponentRegistry(componentRegistry)
	return svcCtx, shutdown, nil
}

// collectorConfigWithAppID 把顶层 app_id 注入 Collector Redis Stream，避免多站点共用 Redis 时串流。
func collectorConfigWithAppID(c config.Config) config.CollectorConfig {
	cfg := c.Collector
	cfg.Redis.Stream = keys.WithPrefix(cfg.Redis.Stream)
	return cfg
}

// Start 启动 HTTP 服务。
func (a *App) Start() error {
	if a == nil || a.Server == nil {
		return errors.Errorf("HTTP 服务未初始化")
	}
	a.startConfigHotReload()
	cfg := a.ServiceContext.CurrentConfig()
	loggerx.Infow(context.Background(), "应用 服务已启动",
		logx.Field("service", cfg.Name),
		logx.Field("host", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		logx.Field("mode", cfg.Mode),
		logx.Field("version", a.ServiceContext.CurrentVersion()),
	)
	a.Server.Start()
	return nil
}

// Stop 释放服务资源。
func (a *App) Stop(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if a.Server != nil {
		a.Server.Stop()
	}
	a.stopConfigHotReload()
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	recordErr(closeServiceContextResources(a.ServiceContext))
	if a.shutdown != nil {
		recordErr(a.shutdown(ctx))
	}
	return firstErr
}

// buildSiteDatabases 初始化默认主库和命名扩展库连接。
func buildSiteDatabases(ctx context.Context, c config.Config) (svc.SiteDatabases, error) {
	if !hasMySQLDataSource(c.MySQL) {
		return svc.SiteDatabases{}, errors.Errorf("缺少 mysql.write_data_source 配置")
	}
	mainDB, err := openSiteDatabase(ctx, "mysql", c.MySQL, c.Observability)
	if err != nil {
		return svc.SiteDatabases{}, errors.Tag(err)
	}
	dbs := svc.SiteDatabases{
		MainDB:   mainDB,
		NamedDBs: make(map[svc.DbName]*gorm.DB),
	}
	names := make([]string, 0, len(c.SiteMySQL))
	for name := range c.SiteMySQL {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dbCfg := c.SiteMySQL[name]
		if !hasMySQLDataSource(dbCfg) {
			continue
		}
		dbName := svc.DbName(strings.TrimSpace(name))
		db, err := openSiteDatabase(ctx, "site_mysql."+string(dbName), dbCfg, c.Observability)
		if err != nil {
			_ = closeSiteDatabases(dbs)
			return svc.SiteDatabases{}, errors.Tag(err)
		}
		dbs.NamedDBs[dbName] = db
	}
	return dbs, nil
}

// openSiteDatabase 校验并打开单个站点数据库连接。
func openSiteDatabase(ctx context.Context, name string, cfg config.MySQLConfig, obs config.ObservabilityConfig) (*gorm.DB, error) {
	if strings.TrimSpace(cfg.WriteDataSource) == "" {
		return nil, errors.Errorf("缺少 %s.write_data_source 配置", name)
	}
	db, err := mysqlx.New(ctx, cfg, obs)
	if err != nil {
		return nil, errors.Wrapf(err, "打开 MySQL[%s]失败", name)
	}
	return db, nil
}

// hasMySQLDataSource 判断 MySQL 配置是否包含写库 DSN。
func hasMySQLDataSource(cfg config.MySQLConfig) bool {
	return strings.TrimSpace(cfg.WriteDataSource) != ""
}

// closeServiceContextResources 按依赖类型释放 ServiceContext 持有的资源。
func closeServiceContextResources(svcCtx *svc.ServiceContext) error {
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	if svcCtx == nil {
		return nil
	}
	if registry := svcCtx.ComponentRegistry(); registry != nil && len(registry.Items()) > 0 {
		return errors.Tag(registry.Close())
	}
	if svcCtx.Rds != nil {
		recordErr(svcCtx.Rds.Close())
	}
	recordErr(closeSiteDatabases(svcCtx.SiteDBs))
	return firstErr
}

// closeSiteDatabases 去重关闭站点数据库连接，避免同一连接池重复关闭。
func closeSiteDatabases(siteDBs svc.SiteDatabases) error {
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	seen := make(map[*gorm.DB]struct{}, 4)
	closeOne := func(name string, db *gorm.DB) {
		if db == nil {
			return
		}
		if _, ok := seen[db]; ok {
			return
		}
		seen[db] = struct{}{}
		sqlDB, err := db.DB()
		if err != nil {
			recordErr(errors.Wrapf(err, "获取 MySQL[%s]底层连接池失败", name))
			return
		}
		if sqlDB == nil {
			return
		}
		if err = sqlDB.Close(); err != nil {
			recordErr(errors.Wrapf(err, "关闭 MySQL[%s]连接池失败", name))
		}
	}
	closeOne("mysql", siteDBs.MainDB)
	for name, db := range siteDBs.NamedDBs {
		closeOne("site_mysql."+string(name), db)
	}
	return firstErr
}
