package database

import (
	"context"
	"strings"

	"github.com/Is999/go-utils/errors"

	"gorm.io/gorm"
)

// insertSchemaMigrationSQL 登记已执行迁移版本，参数必须使用绑定值。
const insertSchemaMigrationSQL = "INSERT INTO schema_migrations (`version`, `name`, `asset`, `checksum`) VALUES (?, ?, ?, ?)"

// GormMigrationStore 使用 GORM 执行迁移 SQL 和版本登记。
type GormMigrationStore struct {
	db *gorm.DB // 写库连接，迁移只允许走主库
}

// NewGormMigrationStore 创建 GORM 迁移存储。
func NewGormMigrationStore(db *gorm.DB) *GormMigrationStore {
	return &GormMigrationStore{db: db}
}

// EnsureSchema 确保 schema_migrations 表存在。
func (s *GormMigrationStore) EnsureSchema(ctx context.Context, schemaSQL string) error {
	if s == nil || s.db == nil {
		return errors.Errorf("数据库迁移 GORM 连接为空")
	}
	if strings.TrimSpace(schemaSQL) == "" {
		return errors.Errorf("schema_migrations DDL 为空")
	}
	if err := s.db.WithContext(ctx).Exec(schemaSQL).Error; err != nil {
		return errors.Wrap(err, "创建 schema_migrations 表失败")
	}
	return nil
}

// AppliedMigrations 读取已登记迁移版本；版本表不存在时按空表处理。
func (s *GormMigrationStore) AppliedMigrations(ctx context.Context) (map[string]AppliedMigration, error) {
	if s == nil || s.db == nil {
		return nil, errors.Errorf("数据库迁移 GORM 连接为空")
	}
	var rows []AppliedMigration
	err := s.db.WithContext(ctx).
		Table("schema_migrations").
		Select("version, name, asset, checksum").
		Scan(&rows).Error
	if err != nil {
		if isMissingMigrationTableError(err) {
			return map[string]AppliedMigration{}, nil
		}
		return nil, errors.Wrap(err, "查询 schema_migrations 失败")
	}
	applied := make(map[string]AppliedMigration, len(rows))
	for _, row := range rows {
		applied[row.Version] = row
	}
	return applied, nil
}

// ExecuteMigration 在事务中执行 SQL 并登记版本。
func (s *GormMigrationStore) ExecuteMigration(ctx context.Context, migration Migration) error {
	if s == nil || s.db == nil {
		return errors.Errorf("数据库迁移 GORM 连接为空")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		statements := splitMigrationStatements(migration.SQL)
		if len(statements) == 0 {
			return errors.Errorf("数据库迁移 SQL 为空 version=%s name=%s asset=%s", migration.Version, migration.Name, migration.Asset)
		}
		for idx, statement := range statements {
			if shouldSkipMigrationStatement(statement) {
				continue
			}
			if err := tx.Exec(statement).Error; err != nil {
				return errors.Wrapf(err, "执行数据库迁移 SQL 失败 version=%s name=%s asset=%s statement=%d", migration.Version, migration.Name, migration.Asset, idx+1)
			}
		}
		if err := tx.Exec(insertSchemaMigrationSQL, migration.Version, migration.Name, migration.Asset, migration.Checksum).Error; err != nil {
			return errors.Wrapf(err, "登记数据库迁移版本失败 version=%s name=%s", migration.Version, migration.Name)
		}
		return nil
	})
}

// isMissingMigrationTableError 判断版本表不存在错误，首次初始化时按空迁移表处理。
func isMissingMigrationTableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "schema_migrations") &&
		(strings.Contains(message, "doesn't exist") || strings.Contains(message, "no such table") || strings.Contains(message, "error 1146"))
}

// splitMigrationStatements 按分号拆分 SQL，同时保留字符串和注释中的分号。
func splitMigrationStatements(sqlText string) []string {
	statements := make([]string, 0)
	var builder strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false
	escaped := false

	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]
		next := byte(0)
		if i+1 < len(sqlText) {
			next = sqlText[i+1]
		}

		if inLineComment {
			builder.WriteByte(ch)
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			builder.WriteByte(ch)
			if ch == '*' && next == '/' {
				builder.WriteByte(next)
				i++
				inBlockComment = false
			}
			continue
		}
		if inSingleQuote {
			builder.WriteByte(ch)
			if ch == '\\' && !escaped {
				escaped = true
				continue
			}
			if ch == '\'' && !escaped {
				if next == '\'' {
					builder.WriteByte(next)
					i++
					continue
				}
				inSingleQuote = false
			}
			escaped = false
			continue
		}
		if inDoubleQuote {
			builder.WriteByte(ch)
			if ch == '\\' && !escaped {
				escaped = true
				continue
			}
			if ch == '"' && !escaped {
				inDoubleQuote = false
			}
			escaped = false
			continue
		}
		if inBacktick {
			builder.WriteByte(ch)
			if ch == '`' {
				inBacktick = false
			}
			continue
		}

		switch {
		case ch == '-' && next == '-':
			builder.WriteByte(ch)
			builder.WriteByte(next)
			i++
			inLineComment = true
		case ch == '#':
			builder.WriteByte(ch)
			inLineComment = true
		case ch == '/' && next == '*':
			builder.WriteByte(ch)
			builder.WriteByte(next)
			i++
			inBlockComment = true
		case ch == '\'':
			builder.WriteByte(ch)
			inSingleQuote = true
		case ch == '"':
			builder.WriteByte(ch)
			inDoubleQuote = true
		case ch == '`':
			builder.WriteByte(ch)
			inBacktick = true
		case ch == ';':
			statement := strings.TrimSpace(builder.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			builder.Reset()
		default:
			builder.WriteByte(ch)
		}
	}
	statement := strings.TrimSpace(builder.String())
	if statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

// shouldSkipMigrationStatement 跳过事务控制语句，迁移执行统一由 GORM 事务包裹。
func shouldSkipMigrationStatement(statement string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(trimLeadingSQLComments(statement)))
	return normalized == "BEGIN" ||
		normalized == "COMMIT" ||
		normalized == "ROLLBACK" ||
		normalized == "START TRANSACTION"
}

// trimLeadingSQLComments 删除语句开头注释，避免注释干扰安全判断。
func trimLeadingSQLComments(statement string) string {
	for {
		statement = strings.TrimSpace(statement)
		switch {
		case strings.HasPrefix(statement, "--"):
			idx := strings.IndexByte(statement, '\n')
			if idx < 0 {
				return ""
			}
			statement = statement[idx+1:]
		case strings.HasPrefix(statement, "#"):
			idx := strings.IndexByte(statement, '\n')
			if idx < 0 {
				return ""
			}
			statement = statement[idx+1:]
		case strings.HasPrefix(statement, "/*"):
			idx := strings.Index(statement, "*/")
			if idx < 0 {
				return statement
			}
			statement = statement[idx+len("*/"):]
		default:
			return statement
		}
	}
}
