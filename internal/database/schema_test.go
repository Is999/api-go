package database

import (
	"strings"
	"testing"
)

// TestUserSchemaSQL 验证业务用户表 DDL 会剥离说明头。
func TestUserSchemaSQL(t *testing.T) {
	sql := UserSchemaSQL()

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("UserSchemaSQL() should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `user`") {
		t.Fatalf("UserSchemaSQL() missing user DDL: %q", sql)
	}
	for _, want := range []string{
		"`id` bigint NOT NULL COMMENT '雪花ID'",
		"`shard_no` int NOT NULL DEFAULT 0 COMMENT 'ID哈希分片，CRC32(id字符串)%1000，用于分表和分片游标查询'",
		"KEY `idx_user_shard_no_id` (`shard_no`, `id`)",
		"KEY `idx_user_status_id` (`status`, `id`)",
		"CONSTRAINT `chk_user_shard_no` CHECK (`shard_no` BETWEEN 0 AND 999)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("UserSchemaSQL() missing %q: %q", want, sql)
		}
	}
	for _, forbidden := range []string{"MOD(`id`, 1000)", "`id` % 1000", "id%1000"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("UserSchemaSQL() should not use id modulo rule %q: %q", forbidden, sql)
		}
	}
}

// TestUserAccountSchemaSQL 验证业务用户全局账号索引表 DDL 结构。
func TestUserAccountSchemaSQL(t *testing.T) {
	sql := UserAccountSchemaSQL()

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("UserAccountSchemaSQL() should strip header comments: %q", sql)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `user_account`",
		"UNIQUE KEY `uk_user_account_username` (`username`)",
		"UNIQUE KEY `uk_user_account_user_id` (`user_id`)",
		"KEY `idx_user_account_shard_user` (`shard_no`, `user_id`)",
		"KEY `idx_user_account_route_shard_user` (`route_shard_count`, `shard_no`, `user_id`)",
		"`route_shard_count` smallint unsigned NOT NULL DEFAULT 1",
		"CONSTRAINT `chk_user_account_shard_no` CHECK (`shard_no` BETWEEN 0 AND 999)",
		"CONSTRAINT `chk_user_account_route_shard_count` CHECK (`route_shard_count` IN (1, 10, 100, 1000))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("UserAccountSchemaSQL() missing %q: %q", want, sql)
		}
	}
}

// TestSysConfigSchemaSQL 验证系统配置表 DDL 使用 sys_config 表名并剥离说明头。
func TestSysConfigSchemaSQL(t *testing.T) {
	sql := SysConfigSchemaSQL()

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("SysConfigSchemaSQL() should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `sys_config`") {
		t.Fatalf("SysConfigSchemaSQL() missing sys_config DDL: %q", sql)
	}
	if strings.Contains(sql, "`api_sys_config`") {
		t.Fatalf("SysConfigSchemaSQL() should not use api_ table prefix: %q", sql)
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
