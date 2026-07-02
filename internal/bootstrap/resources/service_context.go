package resources

import (
	"context"
	"sort"
	"strings"

	"api/internal/bootstrap/configload"
	"api/internal/config"
	"api/internal/infra/loggerx"
	mysqlx "api/internal/infra/mysql"
	"api/internal/infra/redisx"
	"api/internal/infra/tracing"
	"api/internal/svc"

	"github.com/Is999/go-utils/errors"
	"gorm.io/gorm"
)

// buildResources 聚合 BuildServiceContext 启动过程中已成功初始化、但尚未交给 App 托管的资源。
type buildResources struct {
	svc.Dependencies                             // ServiceContext 可直接复用的依赖集合
	Shutdown         func(context.Context) error // tracing 等基础设施关闭钩子，最后释放
}

// BuildServiceContext 统一完成基础设施初始化，避免入口层各自拼装依赖导致行为漂移。
func BuildServiceContext(ctx context.Context, c config.Config, version string) (*svc.ServiceContext, func(context.Context) error, error) {
	loggerx.Setup(c)
	if err := configload.ConfigureSnowflakeWorkerID(c.Snowflake); err != nil {
		return nil, nil, errors.Wrap(err, "配置雪花 ID worker 失败")
	}
	shutdown, err := tracing.Setup(ctx, c.Observability)
	if err != nil {
		return nil, nil, errors.Tag(err)
	}
	resources := buildResources{Dependencies: svc.Dependencies{}, Shutdown: shutdown}

	siteDBs, err := buildSiteDatabases(ctx, c)
	resources.SiteDBs = siteDBs
	if err != nil {
		_ = closeBuildResources(context.Background(), resources)
		return nil, nil, errors.Tag(err)
	}

	rdb, err := redisx.New(ctx, c.Redis, c.Observability)
	if err != nil {
		_ = closeBuildResources(context.Background(), resources)
		return nil, nil, errors.Tag(err)
	}
	resources.Rds = rdb

	svcCtx := svc.NewServiceContext(c, version, resources.Dependencies)
	return svcCtx, shutdown, nil
}

// closeBuildResources 回收 BuildServiceContext 已经创建但尚未交给 App 托管的资源。
func closeBuildResources(ctx context.Context, resources buildResources) error {
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = errors.Tag(err)
		}
	}
	if resources.Rds != nil {
		recordErr(resources.Rds.Close())
	}
	recordErr(closeSiteDatabases(resources.SiteDBs))
	if resources.Shutdown != nil {
		recordErr(resources.Shutdown(ctx))
	}
	return errors.Tag(firstErr)
}

// CloseServiceContextResources 释放 ServiceContext 托管的外部资源。
func CloseServiceContextResources(svcCtx *svc.ServiceContext) error {
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
		NamedDBs: make(map[svc.DBName]*gorm.DB),
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
		dbName := svc.DBName(strings.TrimSpace(name))
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
	return errors.Tag(firstErr)
}
