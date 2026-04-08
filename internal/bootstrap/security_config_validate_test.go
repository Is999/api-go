package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"gozero_api/internal/config"
)

// TestValidateConfigRejectsSecurityWithoutAppID 确保配置安全链路时必须绑定 AppID。
func TestValidateConfigRejectsSecurityWithoutAppID(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:   "v1",
		SignStatus:   0,
		CryptoStatus: 0,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected configured security without app_id to be rejected")
	}
}

// TestValidateConfigRejectsSecurityMissingRSAWhenSignEnabled 确保启用签名时必须配置 RSA 材料。
func TestValidateConfigRejectsSecurityMissingRSAWhenSignEnabled(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:   "v1",
		SignStatus:   1,
		CryptoStatus: 0,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected sign-enabled security without rsa material to be rejected")
	}
}

// TestValidateConfigRejectsSecurityMissingAESWhenCryptoEnabled 确保启用加密时必须配置 AES 材料。
func TestValidateConfigRejectsSecurityMissingAESWhenCryptoEnabled(t *testing.T) {
	userPublicPEM, serverPrivatePEM := testSecurityRSAKeys(t)
	cfg := validBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:          "v1",
		SignStatus:          0,
		CryptoStatus:        1,
		RSAPublicKeyUser:    userPublicPEM,
		RSAPrivateKeyServer: serverPrivatePEM,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected crypto-enabled security without aes material to be rejected")
	}
}

// TestValidateConfigAcceptsSecuritySingleVersion 确保完整单版本秘钥配置可通过启动校验。
func TestValidateConfigAcceptsSecuritySingleVersion(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = validSecuritySecretKey(t, "v1")
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
}

// TestValidateConfigAcceptsSecurityMultiVersionGray 确保多版本稳定和灰度选路配置可通过生产校验。
func TestValidateConfigAcceptsSecurityMultiVersionGray(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:    "v1",
		StableVersion: "v1",
		GrayVersion:   "v2",
		GrayPercent:   50,
		GraySalt:      "prod-gray-salt",
		SignStatus:    1,
		CryptoStatus:  1,
		Versions: []config.SecuritySecretKeyVersionConfig{
			validSecuritySecretKeyVersion(t, "v1"),
			validSecuritySecretKeyVersion(t, "v2"),
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}
}

// TestValidateConfigRejectsSecurityDuplicateVersion 确保重复版本不能通过启动校验。
func TestValidateConfigRejectsSecurityDuplicateVersion(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:   "v1",
		SignStatus:   0,
		CryptoStatus: 0,
		Versions: []config.SecuritySecretKeyVersionConfig{
			{KeyVersion: "v1"},
			{KeyVersion: "v1"},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected duplicate security key version to be rejected")
	}
}

// TestValidateConfigRejectsSecurityUnknownStableVersion 确保稳定版本必须存在于版本材料中。
func TestValidateConfigRejectsSecurityUnknownStableVersion(t *testing.T) {
	cfg := validBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:    "v2",
		StableVersion: "v2",
		SignStatus:    0,
		CryptoStatus:  0,
		Versions: []config.SecuritySecretKeyVersionConfig{
			{KeyVersion: "v1"},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected unknown stable security key version to be rejected")
	}
}

// TestValidateConfigRejectsProductionSecurityGrayWithoutSalt 确保生产灰度秘钥必须配置哈希盐值。
func TestValidateConfigRejectsProductionSecurityGrayWithoutSalt(t *testing.T) {
	cfg := validProductionBootstrapConfig()
	cfg.AppID = "demo-app"
	cfg.Security.SecretKey = config.SecuritySecretKeyConfig{
		KeyVersion:    "v1",
		StableVersion: "v1",
		GrayVersion:   "v2",
		GrayPercent:   10,
		SignStatus:    1,
		CryptoStatus:  1,
		Versions: []config.SecuritySecretKeyVersionConfig{
			validSecuritySecretKeyVersion(t, "v1"),
			validSecuritySecretKeyVersion(t, "v2"),
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected production gray security key without salt to be rejected")
	}
}

// validSecuritySecretKey 构造完整单版本安全配置。
func validSecuritySecretKey(t *testing.T, version string) config.SecuritySecretKeyConfig {
	versionCfg := validSecuritySecretKeyVersion(t, version)
	return config.SecuritySecretKeyConfig{
		KeyVersion:          version,
		SignStatus:          1,
		CryptoStatus:        1,
		AESKey:              versionCfg.AESKey,
		AESIV:               versionCfg.AESIV,
		RSAPublicKeyUser:    versionCfg.RSAPublicKeyUser,
		RSAPrivateKeyServer: versionCfg.RSAPrivateKeyServer,
	}
}

// validSecuritySecretKeyVersion 构造完整版本材料。
func validSecuritySecretKeyVersion(t *testing.T, version string) config.SecuritySecretKeyVersionConfig {
	userPublicPEM, serverPrivatePEM := testSecurityRSAKeys(t)
	return config.SecuritySecretKeyVersionConfig{
		KeyVersion:          version,
		AESKey:              "1234567890123456",
		AESIV:               "abcdefghijklmnop",
		RSAPublicKeyUser:    userPublicPEM,
		RSAPrivateKeyServer: serverPrivatePEM,
	}
}

// testSecurityRSAKeys 生成测试用 RSA 公钥和私钥 PEM。
func testSecurityRSAKeys(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes})
	return string(publicPEM), string(privatePEM)
}
