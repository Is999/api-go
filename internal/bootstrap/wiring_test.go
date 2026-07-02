package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"api/internal/bootstrap/configload"
	"api/internal/config"
)

// TestLoadConfigSampleRequiresProductionSecrets 验证生产示例配置仍会拒绝占位密钥。
func TestLoadConfigSampleRequiresProductionSecrets(t *testing.T) {
	file := filepath.Join("..", "..", "etc", "config.sample.yaml")
	if _, _, err := LoadConfig(file); err == nil {
		t.Fatal("expected production sample with placeholders to be rejected")
	}
}

// TestLoadConfigDNMPSample 验证 DNMP 本地示例配置可加载并生成配置版本。
func TestLoadConfigDNMPSample(t *testing.T) {
	file := filepath.Join("..", "..", "etc", "config.dnmp.sample.yaml")
	cfg, version, err := LoadConfig(file)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Name == "" {
		t.Fatal("config name should not be empty")
	}
	if version == "" {
		t.Fatal("config version should not be empty")
	}
}

// TestNormalizeConfigUsesModeForObservability 确保观测环境复用顶层 Mode，不维护第二套环境。
func TestNormalizeConfigUsesModeForObservability(t *testing.T) {
	cfg := config.Config{
		Observability: config.ObservabilityConfig{
			Environment: "custom-env",
		},
	}
	cfg.Mode = "pro"

	configload.Normalize(&cfg)

	if cfg.Observability.Environment != "pro" {
		t.Fatalf("期望观测环境复用 Mode，实际为 %q", cfg.Observability.Environment)
	}
}

// TestLoadConfigMergesRuntimeConfigFile 验证主配置可合并外置运行时配置文件。
func TestLoadConfigMergesRuntimeConfigFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config.d"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mainFile := filepath.Join(dir, "config.yaml")
	runtimeFile := filepath.Join(dir, "config.d", "runtime.yaml")
	if err := os.WriteFile(mainFile, []byte(`
Name: "api"
Host: "0.0.0.0"
Port: 8890
Mode: "dev"
app_id: "1"
snowflake:
  worker_id: 1
jwt_secret: "test-secret-please-change"
auth:
  password_min_length: 8
hot_reload:
  enabled: false
security:
  secret_key:
    sign_status: 1
    crypto_status: 1
config_files:
  runtime: "config.d/runtime.yaml"
redis:
  addrs:
    - "127.0.0.1:6379"
  password: ""
  db: 0
  pool_size: 1
`), 0o644); err != nil {
		t.Fatalf("WriteFile(main) error = %v", err)
	}
	if err := os.WriteFile(runtimeFile, []byte(`
auth:
  password_min_length: 12
hot_reload:
  enabled: true
  check_interval_seconds: 9
security:
  secret_key:
    sign_status: 0
    crypto_status: 0
    gray_percent: 7
collector:
  enabled: true
  transport: "sync"
ops:
  config_reload_token: "runtime-token"
unknown_block:
  ignored: true
`), 0o644); err != nil {
		t.Fatalf("WriteFile(runtime) error = %v", err)
	}

	cfg, _, err := LoadConfig(mainFile)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Auth.PasswordMinLength != 12 {
		t.Fatalf("password_min_length = %d, want 12", cfg.Auth.PasswordMinLength)
	}
	if !cfg.HotReload.Enabled || cfg.HotReload.CheckIntervalSeconds != 9 {
		t.Fatalf("hot_reload config not merged: %+v", cfg.HotReload)
	}
	if cfg.Security.SecretKey.SignStatus != 0 || cfg.Security.SecretKey.CryptoStatus != 0 || cfg.Security.SecretKey.GrayPercent != 7 {
		t.Fatalf("security config not merged: %+v", cfg.Security)
	}
	if !cfg.Collector.Enabled || cfg.Collector.Transport != "sync" {
		t.Fatalf("collector config not merged: %+v", cfg.Collector)
	}
	if cfg.Ops.ConfigReloadToken != "runtime-token" {
		t.Fatalf("ops config not merged: %+v", cfg.Ops)
	}
}

// TestConfigBundleFingerprintIncludesRuntimeFile 验证配置包指纹会纳入外置运行时配置文件。
func TestConfigBundleFingerprintIncludesRuntimeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config.d"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mainFile := filepath.Join(dir, "config.yaml")
	runtimeFile := filepath.Join(dir, "config.d", "runtime.yaml")
	if err := os.WriteFile(mainFile, []byte(`
Name: "api"
Host: "0.0.0.0"
Port: 8890
Mode: "dev"
jwt_secret: "test-secret-please-change"
config_files:
  runtime: "config.d/runtime.yaml"
redis:
  addrs:
    - "127.0.0.1:6379"
  password: ""
  db: 0
  pool_size: 1
`), 0o644); err != nil {
		t.Fatalf("WriteFile(main) error = %v", err)
	}
	if err := os.WriteFile(runtimeFile, []byte("collector:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(runtime first) error = %v", err)
	}
	first, err := configload.BundleFingerprint(mainFile)
	if err != nil {
		t.Fatalf("BundleFingerprint(first) error = %v", err)
	}
	if err := os.WriteFile(runtimeFile, []byte("collector:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(runtime second) error = %v", err)
	}
	second, err := configload.BundleFingerprint(mainFile)
	if err != nil {
		t.Fatalf("BundleFingerprint(second) error = %v", err)
	}
	if first == second {
		t.Fatal("runtime file change should update bundle fingerprint")
	}
}
