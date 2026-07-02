package database

import (
	"strings"
	"testing"
)

// TestUserSchemaAsset 验证业务用户表 DDL 会剥离说明头。
func TestUserSchemaAsset(t *testing.T) {
	sql := readMigrationSQL(userSchemaAsset)

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("readMigrationSQL(userSchemaAsset) should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `user`") {
		t.Fatalf("readMigrationSQL(userSchemaAsset) missing user DDL: %q", sql)
	}
	for _, want := range []string{
		"`id` bigint NOT NULL COMMENT '雪花ID'",
		"`shard_no` int NOT NULL DEFAULT 0 COMMENT 'ID哈希分片，CRC32(id字符串)%1024，用于分表和分片游标查询'",
		"KEY `idx_user_shard_no_id` (`shard_no`, `id`)",
		"KEY `idx_user_status_id` (`status`, `id`)",
		"CONSTRAINT `chk_user_shard_no` CHECK (`shard_no` BETWEEN 0 AND 1023)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("readMigrationSQL(userSchemaAsset) missing %q: %q", want, sql)
		}
	}
	for _, forbidden := range []string{"MOD(`id`, 1024)", "`id` % 1024", "id%1024"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("readMigrationSQL(userSchemaAsset) should not use id modulo rule %q: %q", forbidden, sql)
		}
	}
}

// TestUserAccountSchemaAsset 验证业务用户全局账号索引表 DDL 结构。
func TestUserAccountSchemaAsset(t *testing.T) {
	sql := readMigrationSQL(userAccountSchemaAsset)

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("readMigrationSQL(userAccountSchemaAsset) should strip header comments: %q", sql)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `user_account`",
		"UNIQUE KEY `uk_user_account_username` (`username`)",
		"UNIQUE KEY `uk_user_account_user_id` (`user_id`)",
		"KEY `idx_user_account_shard_user` (`shard_no`, `user_id`)",
		"KEY `idx_user_account_route_shard_user` (`route_shard_count`, `shard_no`, `user_id`)",
		"`route_shard_count` smallint unsigned NOT NULL DEFAULT 1",
		"CONSTRAINT `chk_user_account_shard_no` CHECK (`shard_no` BETWEEN 0 AND 1023)",
		"CONSTRAINT `chk_user_account_route_shard_count` CHECK (`route_shard_count` IN (1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("readMigrationSQL(userAccountSchemaAsset) missing %q: %q", want, sql)
		}
	}
}

// TestSysConfigSchemaAsset 验证系统配置表 DDL 使用 sys_config 表名并剥离说明头。
func TestSysConfigSchemaAsset(t *testing.T) {
	sql := readMigrationSQL(sysConfigSchemaAsset)

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("readMigrationSQL(sysConfigSchemaAsset) should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `sys_config`") {
		t.Fatalf("readMigrationSQL(sysConfigSchemaAsset) missing sys_config DDL: %q", sql)
	}
	if strings.Contains(sql, "`api_sys_config`") {
		t.Fatalf("readMigrationSQL(sysConfigSchemaAsset) should not use api_ table prefix: %q", sql)
	}
}

// TestSchemaMigrationsSQL 验证迁移版本表 DDL 会剥离说明头。
func TestSchemaMigrationsSQL(t *testing.T) {
	sql := SchemaMigrationsSQL()

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("SchemaMigrationsSQL() should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `schema_migrations`") {
		t.Fatalf("SchemaMigrationsSQL() missing schema_migrations DDL: %q", sql)
	}
}
