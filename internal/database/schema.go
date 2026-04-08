package database

import (
	_ "embed"

	"gozero_api/common/embedasset"
)

// apiUserSchemaSQL 保存前台用户表 DDL 模板。
//
//go:embed api_user_schema.sql.tmpl
var apiUserSchemaSQL string

// sysConfigSchemaSQL 保存系统配置表 DDL 模板。
//
//go:embed sys_config_schema.sql.tmpl
var sysConfigSchemaSQL string

// schemaMigrationsSQL 保存数据库迁移版本表 DDL 模板。
//
//go:embed schema_migrations.sql.tmpl
var schemaMigrationsSQL string

// APIUserSchemaSQL 返回剥离文件头说明后的前台用户表 DDL。
func APIUserSchemaSQL() string {
	return embedasset.StripLeadingLineComments(apiUserSchemaSQL, "--")
}

// SysConfigSchemaSQL 返回剥离文件头说明后的系统配置表 DDL。
func SysConfigSchemaSQL() string {
	return embedasset.StripLeadingLineComments(sysConfigSchemaSQL, "--")
}

// SchemaMigrationsSQL 返回剥离文件头说明后的迁移版本表 DDL。
func SchemaMigrationsSQL() string {
	return embedasset.StripLeadingLineComments(schemaMigrationsSQL, "--")
}
