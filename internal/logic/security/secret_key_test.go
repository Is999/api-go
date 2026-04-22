package security

import (
	"context"
	"testing"

	"api/internal/config"
	"api/internal/svc"
)

func TestSecretKeyLogicUsesConfigVersion(t *testing.T) {
	cfg := config.Config{
		AppID: "demo-app",
		Security: config.SecurityConfig{
			SecretKey: config.SecuritySecretKeyConfig{
				KeyVersion:   "v1",
				SignStatus:   1,
				CryptoStatus: 1,
				AESKey:       "1234567890123456",
				AESIV:        "abcdefghijklmnop",
			},
		},
	}
	svcCtx := svc.NewServiceContext(cfg, "test-version", svc.Dependencies{})
	logic := NewSecretKeyLogic(context.Background(), svcCtx)

	route, err := logic.GetRouteConfig("demo-app")
	if err != nil {
		t.Fatalf("GetRouteConfig() error = %v", err)
	}
	if !route.SignEnabled() || !route.CryptoEnabled() {
		t.Fatalf("route switch should be enabled: %+v", route)
	}

	key, version, err := logic.GetAESKey("demo-app", "", "user-1")
	if err != nil {
		t.Fatalf("GetAESKey() error = %v", err)
	}
	if version != "v1" {
		t.Fatalf("version = %q, want v1", version)
	}
	if key.Key != "1234567890123456" || key.IV != "abcdefghijklmnop" {
		t.Fatalf("unexpected AES key: %+v", key)
	}
}

func TestSecretKeyLogicRejectsWrongAppID(t *testing.T) {
	cfg := config.Config{
		AppID: "demo-app",
		Security: config.SecurityConfig{
			SecretKey: config.SecuritySecretKeyConfig{KeyVersion: "v1"},
		},
	}
	svcCtx := svc.NewServiceContext(cfg, "test-version", svc.Dependencies{})
	logic := NewSecretKeyLogic(context.Background(), svcCtx)

	if _, err := logic.GetRouteConfig("other-app"); err == nil {
		t.Fatal("GetRouteConfig() expected error for wrong app id")
	}
}
