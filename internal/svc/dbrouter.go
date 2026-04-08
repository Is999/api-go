package svc

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// DbName 表示可路由的数据库名称。
type DbName string

// 数据库名称枚举，空值会归一化到主库。
const (
	// DatabaseMain 表示默认主库。
	DatabaseMain DbName = "main"
)

// DB 根据数据库名称返回默认连接。
func (s *ServiceContext) DB(database DbName) *gorm.DB {
	if s == nil {
		return nil
	}
	return s.SiteDBs.Lookup(database)
}

// ReadDB 根据数据库名称返回只读连接。
func (s *ServiceContext) ReadDB(database DbName) *gorm.DB {
	if s == nil {
		return nil
	}
	return readDB(s.SiteDBs.Lookup(database))
}

// WriteDB 根据数据库名称返回写连接。
func (s *ServiceContext) WriteDB(database DbName) *gorm.DB {
	if s == nil {
		return nil
	}
	return writeDB(s.SiteDBs.Lookup(database))
}

// NormalizeDbName 规范化数据库名称，空值统一回退主库。
func NormalizeDbName(database DbName) DbName {
	name := strings.TrimSpace(string(database))
	if name == "" || strings.EqualFold(name, string(DatabaseMain)) {
		return DatabaseMain
	}
	return DbName(name)
}

// readDB 返回强制走读连接的 GORM 会话。
func readDB(db *gorm.DB) *gorm.DB {
	if db == nil || db.Statement == nil {
		return db
	}
	return db.Clauses(dbresolver.Read)
}

// writeDB 返回强制走写连接的 GORM 会话。
func writeDB(db *gorm.DB) *gorm.DB {
	if db == nil || db.Statement == nil {
		return db
	}
	return db.Clauses(dbresolver.Write)
}
