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

// migrationSpec 描述内置迁移资产元数据。
type migrationSpec struct {
	version       string // 迁移版本号
	name          string // 迁移名称
	asset         string // SQL 模板资产文件名
	bootstrapOnly bool   // 是否仅用于新库初始化
	destructive   bool   // 是否含破坏性语句
}

// defaultMigrationSpecs 定义内置迁移清单，顺序即执行顺序。
var defaultMigrationSpecs = []migrationSpec{
	{version: "202606220001", name: "create_user", asset: userSchemaAsset},
	{version: "202606220002", name: "create_sys_config", asset: sysConfigSchemaAsset},
	{version: "202606220003", name: "create_user_identity_username", asset: userIdentityUsernameSchemaAsset},
	{version: "202606220004", name: "create_user_identity_email", asset: userIdentityEmailSchemaAsset},
	{version: "202606220005", name: "create_user_identity_phone", asset: userIdentityPhoneSchemaAsset},
	{version: "202606220006", name: "create_user_identity_oauth", asset: userIdentityOAuthSchemaAsset},
}

// DefaultMigrations 返回内置数据库迁移清单。
func DefaultMigrations() []Migration {
	items := make([]Migration, 0, len(defaultMigrationSpecs))
	for _, spec := range defaultMigrationSpecs {
		items = append(items, newMigrationFromSpec(spec))
	}
	return items
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

// newMigrationFromSpec 从迁移规格生成带摘要的迁移项。
func newMigrationFromSpec(spec migrationSpec) Migration {
	item := newMigration(spec.version, spec.name, spec.asset, readMigrationSQL(spec.asset))
	item.BootstrapOnly = spec.bootstrapOnly
	item.Destructive = spec.destructive
	return item
}

// sha256Hex 返回文本 SHA256 十六进制摘要。
func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
