package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigSampleRequiresProductionSecrets(t *testing.T) {
	file := filepath.Join("..", "..", "etc", "config.sample.yaml")
	if _, _, err := LoadConfig(file); err == nil {
		t.Fatal("expected production sample with placeholders to be rejected")
	}
}

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
jwt_secret: "test-secret-please-change"
auth:
  password_min_length: 8
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
	if !cfg.Collector.Enabled || cfg.Collector.Transport != "sync" {
		t.Fatalf("collector config not merged: %+v", cfg.Collector)
	}
	if cfg.Ops.ConfigReloadToken != "runtime-token" {
		t.Fatalf("ops config not merged: %+v", cfg.Ops)
	}
}

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
	first, err := configBundleFingerprint(mainFile)
	if err != nil {
		t.Fatalf("configBundleFingerprint(first) error = %v", err)
	}
	if err := os.WriteFile(runtimeFile, []byte("collector:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(runtime second) error = %v", err)
	}
	second, err := configBundleFingerprint(mainFile)
	if err != nil {
		t.Fatalf("configBundleFingerprint(second) error = %v", err)
	}
	if first == second {
		t.Fatal("runtime file change should update bundle fingerprint")
	}
}
