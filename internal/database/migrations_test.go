package database

import "testing"

// TestDefaultMigrationsValid 确保默认迁移清单完整、版本递增且资产存在。
func TestDefaultMigrationsValid(t *testing.T) {
	if err := validateMigrationList(DefaultMigrations()); err != nil {
		t.Fatalf("validateMigrationList(DefaultMigrations()) error = %v", err)
	}
}

// TestDefaultMigrationsContainCoreTables 确保前台核心表进入迁移清单。
func TestDefaultMigrationsContainCoreTables(t *testing.T) {
	migrations := DefaultMigrations()
	if len(migrations) == 0 {
		t.Fatal("DefaultMigrations() 不能为空")
	}
	for _, item := range migrations {
		if len(item.Checksum) != 64 {
			t.Fatalf("migration checksum length = %d, want 64: %+v", len(item.Checksum), item)
		}
	}
	if migrations[0].Name != "create_user" || migrations[1].Name != "create_sys_config" || migrations[2].Name != "create_user_account" {
		t.Fatalf("DefaultMigrations() order mismatch: %+v", migrations)
	}
	if migrations[0].Version != "202606220001" || migrations[1].Version != "202606220002" || migrations[2].Version != "202606220003" {
		t.Fatalf("DefaultMigrations() version mismatch: %+v", migrations)
	}
}

// TestDefaultMigrationsCoverDatabaseSQLAssets 确保业务 DDL 资产都纳入默认迁移规格。
func TestDefaultMigrationsCoverDatabaseSQLAssets(t *testing.T) {
	assets, err := MigrationAssetNames()
	if err != nil {
		t.Fatalf("MigrationAssetNames() error = %v", err)
	}
	covered := make(map[string]struct{}, len(DefaultMigrations()))
	for _, item := range DefaultMigrations() {
		covered[item.Asset] = struct{}{}
	}
	for _, asset := range assets {
		if asset == schemaMigrationsAsset {
			continue
		}
		if _, ok := covered[asset]; !ok {
			t.Fatalf("database SQL asset missing migration spec: %s", asset)
		}
	}
}

// TestPendingMigrations 确保已登记版本不会再次进入待执行列表。
func TestPendingMigrations(t *testing.T) {
	migrations := DefaultMigrations()
	pending := PendingMigrations(map[string]struct{}{migrations[0].Version: {}})
	if len(pending) != len(migrations)-1 {
		t.Fatalf("PendingMigrations() len = %d, want %d", len(pending), len(migrations)-1)
	}
	if pending[0].Version == migrations[0].Version {
		t.Fatalf("applied migration still pending: %+v", pending[0])
	}
}
