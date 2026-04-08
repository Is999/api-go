package database

import (
	"strings"
	"testing"
)

// TestAPIUserSchemaSQL 验证前台用户表 DDL 会剥离说明头。
func TestAPIUserSchemaSQL(t *testing.T) {
	sql := APIUserSchemaSQL()

	if strings.Contains(sql, "代码资产") {
		t.Fatalf("APIUserSchemaSQL() should strip header comments: %q", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS `api_user`") {
		t.Fatalf("APIUserSchemaSQL() missing api_user DDL: %q", sql)
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
