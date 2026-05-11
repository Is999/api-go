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
	if len(migrations) != len(defaultMigrationSpecs) {
		t.Fatalf("DefaultMigrations() len = %d, want %d", len(migrations), len(defaultMigrationSpecs))
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

// TestDefaultMigrationsMatchSpecs 确保默认迁移清单从规格派生，避免资产和版本信息两头维护。
func TestDefaultMigrationsMatchSpecs(t *testing.T) {
	migrations := DefaultMigrations()
	if len(migrations) != len(defaultMigrationSpecs) {
		t.Fatalf("migration count = %d, spec count = %d", len(migrations), len(defaultMigrationSpecs))
	}
	for index, spec := range defaultMigrationSpecs {
		item := migrations[index]
		if item.Version != spec.version || item.Name != spec.name || item.Asset != spec.asset {
			t.Fatalf("migration mismatch index=%d item=%+v spec=%+v", index, item, spec)
		}
		if item.BootstrapOnly != spec.bootstrapOnly || item.Destructive != spec.destructive {
			t.Fatalf("migration safety flags mismatch index=%d item=%+v spec=%+v", index, item, spec)
		}
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
	if len(pending) != 1 {
		t.Fatalf("PendingMigrations() len = %d, want 1", len(pending))
	}
	if pending[0].Version == migrations[0].Version {
		t.Fatalf("applied migration still pending: %+v", pending[0])
	}
}
