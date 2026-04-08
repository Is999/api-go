package mysqlx

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gozero_api/internal/config"
	"gozero_api/internal/infra/loggerx"

	"github.com/Is999/go-utils/errors"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// New 创建带统一 GORM 日志器的数据库连接，并在启动阶段完成连通性检查。
func New(ctx context.Context, cfg config.MySQLConfig, obs config.ObservabilityConfig) (*gorm.DB, error) {
	writeDSN, readDSNs, err := resolveDataSources(cfg)
	if err != nil {
		return nil, errors.Tag(err)
	}
	if err := checkMySQLDataSources(ctx, writeDSN, readDSNs); err != nil {
		return nil, errors.Tag(err)
	}

	gormCfg := &gorm.Config{
		Logger: loggerx.NewGormLogger(time.Duration(obs.SlowSQLMs) * time.Millisecond),
	}
	gdb, err := gorm.Open(gormmysql.Open(writeDSN), gormCfg)
	if err != nil {
		return nil, errors.Tag(err)
	}

	if len(readDSNs) > 0 {
		replicas := make([]gorm.Dialector, 0, len(readDSNs))
		for _, dsn := range readDSNs {
			replicas = append(replicas, gormmysql.Open(dsn))
		}
		resolver := dbresolver.Register(dbresolver.Config{
			Sources:           []gorm.Dialector{gormmysql.Open(writeDSN)},
			Replicas:          replicas,
			Policy:            dbresolver.RandomPolicy{},
			TraceResolverMode: true,
		})
		if cfg.MaxOpenConns > 0 {
			resolver = resolver.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.MaxIdleConns > 0 {
			resolver = resolver.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			resolver = resolver.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
		}
		if err := gdb.Use(resolver); err != nil {
			closeGormDB(gdb)
			return nil, errors.Wrap(err, "注册 MySQL 读写分离解析器失败")
		}
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		closeGormDB(gdb)
		return nil, errors.Tag(err)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, errors.Tag(err)
	}
	if cfg.Debug {
		gdb = gdb.Debug()
	}
	return gdb, nil
}

// closeGormDB 关闭 GORM 底层连接池，忽略关闭阶段不可恢复错误。
func closeGormDB(gdb *gorm.DB) {
	if gdb == nil {
		return
	}
	sqlDB, err := gdb.DB()
	if err != nil || sqlDB == nil {
		return
	}
	_ = sqlDB.Close()
}

// resolveDataSources 清洗写库和读库 DSN，读库会去除空值、重复值和写库自身。
func resolveDataSources(cfg config.MySQLConfig) (string, []string, error) {
	writeDSN := strings.TrimSpace(cfg.WriteDataSource)
	if writeDSN == "" {
		return "", nil, errors.Errorf("缺少 mysql.write_data_source 配置")
	}
	if len(cfg.ReadDataSources) == 0 {
		return writeDSN, nil, nil
	}
	replicas := make([]string, 0, len(cfg.ReadDataSources))
	seen := map[string]struct{}{}
	for _, dsn := range cfg.ReadDataSources {
		trimmed := strings.TrimSpace(dsn)
		if trimmed == "" || trimmed == writeDSN {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		replicas = append(replicas, trimmed)
	}
	return writeDSN, replicas, nil
}

// checkMySQLDataSources 在启动期探测所有 MySQL DSN。
func checkMySQLDataSources(ctx context.Context, writeDSN string, readDSNs []string) error {
	return checkMySQLDataSourcesWithPing(ctx, writeDSN, readDSNs, pingMySQLDataSource)
}

// checkMySQLDataSourcesWithPing 注入探测函数，便于单测覆盖启动探测分支。
func checkMySQLDataSourcesWithPing(ctx context.Context, writeDSN string, readDSNs []string, ping func(context.Context, string, string) error) error {
	if ping == nil {
		return errors.Errorf("MySQL 启动探测函数不能为空")
	}
	if err := ping(ctx, "write_data_source", writeDSN); err != nil {
		return errors.Tag(err)
	}
	for idx, dsn := range readDSNs {
		if err := ping(ctx, fmt.Sprintf("read_data_sources[%d]", idx), dsn); err != nil {
			return errors.Tag(err)
		}
	}
	return nil
}

// pingMySQLDataSource 使用最小连接池探测单个 MySQL DSN。
func pingMySQLDataSource(ctx context.Context, label, dsn string) error {
	if err := validateMySQLDataSourceDatabase(label, dsn); err != nil {
		return errors.Tag(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return errors.Wrapf(err, "打开 MySQL %s 失败", label)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err := db.PingContext(ctx); err != nil {
		return errors.Wrapf(err, "探测 MySQL %s 失败，数据库不存在或不可达", label)
	}
	return nil
}

// validateMySQLDataSourceDatabase 要求 DSN 显式包含库名，避免误连默认库。
func validateMySQLDataSourceDatabase(label, dsn string) error {
	parsed, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		return errors.Wrapf(err, "解析 MySQL %s DSN 失败", label)
	}
	if strings.TrimSpace(parsed.DBName) == "" {
		return errors.Errorf("MySQL %s DSN 必须包含数据库名", label)
	}
	return nil
}
