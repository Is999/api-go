package database

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Migration 描述一个数据库迁移资产。
type Migration struct {
	Version       string // 迁移版本号，必须单调递增
	Name          string // 迁移名称，必须唯一
	Asset         string // SQL 资产文件名
	SQL           string // 剥离说明后的 SQL 文本
	Checksum      string // SQL 文本 SHA256
	BootstrapOnly bool   // 是否仅允许新库初始化时人工执行
	Destructive   bool   // 是否包含 DROP/种子数据等不适合在线执行的语句
}

// DefaultMigrations 返回内置数据库迁移清单。
func DefaultMigrations() []Migration {
	return []Migration{
		newMigration("202606050001", "create_api_user", "api_user_schema.sql.tmpl", APIUserSchemaSQL()),
		newMigration("202606050002", "create_sys_config", "sys_config_schema.sql.tmpl", SysConfigSchemaSQL()),
	}
}

// PendingMigrations 返回尚未在版本表中登记的迁移。
func PendingMigrations(applied map[string]struct{}) []Migration {
	migrations := DefaultMigrations()
	pending := make([]Migration, 0, len(migrations))
	for _, item := range migrations {
		if _, ok := applied[item.Version]; ok {
			continue
		}
		pending = append(pending, item)
	}
	return pending
}

// ValidateDefaultMigrations 校验默认迁移清单完整性。
func ValidateDefaultMigrations() error {
	return validateMigrationList(DefaultMigrations())
}

// newMigration 创建带摘要的迁移项。
func newMigration(version string, name string, asset string, sqlText string) Migration {
	sqlText = strings.TrimSpace(sqlText)
	return Migration{
		Version:  version,
		Name:     name,
		Asset:    asset,
		SQL:      sqlText,
		Checksum: sha256Hex(sqlText),
	}
}

// sha256Hex 返回文本 SHA256 十六进制摘要。
func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
