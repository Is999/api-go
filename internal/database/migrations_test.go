package database

import "testing"

// TestValidateDefaultMigrations 确保默认迁移清单完整、版本递增且资产存在。
func TestValidateDefaultMigrations(t *testing.T) {
	if err := ValidateDefaultMigrations(); err != nil {
		t.Fatalf("ValidateDefaultMigrations() error = %v", err)
	}
}

// TestDefaultMigrationsContainCoreTables 确保前台核心表进入迁移清单。
func TestDefaultMigrationsContainCoreTables(t *testing.T) {
	migrations := DefaultMigrations()
	if len(migrations) != 2 {
		t.Fatalf("DefaultMigrations() len = %d, want 2", len(migrations))
	}
	for _, item := range migrations {
		if len(item.Checksum) != 64 {
			t.Fatalf("migration checksum length = %d, want 64: %+v", len(item.Checksum), item)
		}
	}
	if migrations[0].Name != "create_api_user" || migrations[1].Name != "create_sys_config" {
		t.Fatalf("DefaultMigrations() order mismatch: %+v", migrations)
	}
}

// TestPendingMigrations 确保已登记版本不会再次进入待执行列表。
func TestPendingMigrations(t *testing.T) {
	migrations := DefaultMigrations()
	pending := PendingMigrations(map[string]struct{}{migrations[0].Version: {}})
	if len(pending) != 1 {
		t.Fatalf("PendingMigrations() len = %d, want 1", len(pending))
	}
	if pending[0].Version == migrations[0].Version {
		t.Fatalf("applied migration still pending: %+v", pending[0])
	}
}
