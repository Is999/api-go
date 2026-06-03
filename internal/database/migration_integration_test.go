//go:build integration

package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const integrationMySQLDSNEnv = "INTEGRATION_MYSQL_DSN" // integrationMySQLDSNEnv 表示集成测试使用的 MySQL DSN 环境变量。

// TestAPIMigrationRunWithMySQL 使用真实 MySQL 校验迁移执行、幂等和 checksum 防篡改。
func TestAPIMigrationRunWithMySQL(t *testing.T) {
	db := openIntegrationMySQL(t)
	resetIntegrationTables(t, db, "schema_migrations", "user", "user_account", "sys_config")
	store := NewGormMigrationStore(db)

	results, err := RunMigrations(context.Background(), store, DefaultMigrations(), MigrationRunOptions{})
	if err != nil {
		t.Fatalf("RunMigrations(up) error = %v", err)
	}
	assertMigrationStatus(t, results, MigrationStatusExecuted)

	results, err = RunMigrations(context.Background(), store, DefaultMigrations(), MigrationRunOptions{})
	if err != nil {
		t.Fatalf("RunMigrations(idempotent) error = %v", err)
	}
	assertMigrationStatus(t, results, MigrationStatusApplied)

	tampered := DefaultMigrations()
	tampered[0].Checksum = strings.Repeat("0", 64)
	if _, err = RunMigrations(context.Background(), store, tampered, MigrationRunOptions{DryRun: true}); err == nil {
		t.Fatal("期望 checksum 不一致返回错误，实际为 nil")
	}
}

// assertMigrationStatus 校验所有迁移执行结果符合期望状态。
func assertMigrationStatus(t *testing.T, results []MigrationRunItem, status string) {
	t.Helper()
	if len(results) != len(DefaultMigrations()) {
		t.Fatalf("迁移结果数量 = %d, want %d", len(results), len(DefaultMigrations()))
	}
	for _, item := range results {
		if item.Status != status {
			t.Fatalf("迁移状态 = %s, want %s: %+v", item.Status, status, item)
		}
	}
}

// openIntegrationMySQL 打开集成测试 MySQL，并在未配置 DSN 时跳过。
func openIntegrationMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(integrationMySQLDSNEnv))
	if dsn == "" {
		t.Skipf("%s 未配置，跳过 MySQL 集成测试", integrationMySQLDSNEnv)
	}
	var lastErr error
	for i := 0; i < 30; i++ {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil && sqlDB.Ping() == nil {
				t.Cleanup(func() { _ = sqlDB.Close() })
				return db
			}
			if dbErr != nil {
				lastErr = dbErr
			}
			if sqlDB != nil {
				_ = sqlDB.Close()
			}
		} else {
			lastErr = err
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("连接集成 MySQL 失败: %v", lastErr)
	return nil
}

// resetIntegrationTables 清理迁移集成测试涉及的表，保证每轮执行从空库开始。
func resetIntegrationTables(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Fatalf("drop integration table %s: %v", table, err)
		}
	}
}
