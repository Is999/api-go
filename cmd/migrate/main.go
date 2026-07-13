package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"api/internal/bootstrap"
	"api/internal/database"
	mysqlx "api/internal/infra/mysql"

	"github.com/Is999/go-utils/errors"
)

// 迁移动作常量限定命令行允许的执行模式。
const (
	actionStatus      = "status"               // 只查看迁移状态
	actionDryRun      = "dry-run"              // 预览迁移计划但不执行 SQL
	actionUp          = "up"                   // 执行允许范围内的待迁移 SQL
	migrationLockName = "app:schema-migration" // 与 admin 共用迁移锁，避免同实例并发修改库结构
	migrationLockWait = time.Minute            // 发布任务等待已有迁移结束的最大时间
)

// buildVersion 由构建阶段通过 -ldflags 注入，用于发布排查。
var buildVersion = "dev"

// main 解析命令行参数并执行数据库迁移命令。
func main() {
	configFile := flag.String("f", "./etc/config.yaml", "配置文件路径")
	action := flag.String("action", actionStatus, "迁移动作：status/dry-run/up")
	allowBootstrap := flag.Bool("allow-bootstrap", false, "允许执行 bootstrap-only 基线迁移")
	allowDestructive := flag.Bool("allow-destructive", false, "允许执行 destructive 迁移")
	showVersion := flag.Bool("version", false, "输出构建版本并退出")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildVersion)
		return
	}

	if err := run(*configFile, *action, *allowBootstrap, *allowDestructive); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 加载配置、连接主库，并按指定动作执行或预览迁移。
func run(configFile string, action string, allowBootstrap bool, allowDestructive bool) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != actionStatus && action != actionDryRun && action != actionUp {
		return errors.Errorf("不支持的迁移动作: %s", action)
	}
	ctx := context.Background()
	cfg, _, err := bootstrap.LoadConfig(configFile)
	if err != nil {
		return errors.Wrap(err, "加载配置失败")
	}
	db, err := mysqlx.New(ctx, cfg.MySQL, cfg.Observability)
	if err != nil {
		return errors.Wrap(err, "连接 MySQL 失败")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.Wrap(err, "获取 MySQL 底层连接失败")
	}
	defer sqlDB.Close()

	var results []database.MigrationRunItem
	err = database.WithMigrationLock(ctx, sqlDB, migrationLockName, migrationLockWait, func() error {
		var runErr error
		results, runErr = database.RunMigrations(ctx, database.NewGormMigrationStore(db), database.DefaultMigrations(), database.MigrationRunOptions{
			DryRun:           action != actionUp,
			AllowBootstrap:   allowBootstrap,
			AllowDestructive: allowDestructive,
		})
		return errors.Tag(runErr)
	})
	printResults(results)
	return errors.Tag(err)
}

// printResults 以固定列宽输出迁移状态，便于发布脚本读取。
func printResults(results []database.MigrationRunItem) {
	fmt.Printf("%-10s %-14s %-36s %s\n", "STATUS", "VERSION", "NAME", "ASSET")
	for _, item := range results {
		line := fmt.Sprintf("%-10s %-14s %-36s %s", item.Status, item.Version, item.Name, item.Asset)
		if item.Reason != "" {
			line += " # " + item.Reason
		}
		fmt.Println(line)
	}
}
