package logic

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"hash/fnv"
	"os"
	"strings"

	"gozero_api/internal/config"
	"gozero_api/internal/security"
	"gozero_api/internal/svc"

	"github.com/Is999/go-utils/errors"
)

const (
	// RSAUserPublicKey 表示用户 RSA 公钥，用于请求验签和响应加密。
	RSAUserPublicKey = "user_public_key"
	// RSAServerPublicKey 表示服务端 RSA 公钥，用于对外展示。
	RSAServerPublicKey = "server_public_key"
	// RSAServerPrivateKey 表示服务端 RSA 私钥，用于请求解密和响应签名。
	RSAServerPrivateKey = "server_private_key"
)

// AESKey 表示启用状态的 AES KEY 与 IV。
type AESKey struct {
	Key string // Key 是 AES 密钥明文，长度必须为 16、24 或 32 位
	IV  string // IV 是 AES CBC 初始化向量，长度必须为 16 位
}

// SecretKeyRouteConfig 表示运行时版本选路配置。
type SecretKeyRouteConfig struct {
	UUID          string // UUID 表示接入应用 AppID
	StableVersion string // StableVersion 表示稳定版本号
	GrayVersion   string // GrayVersion 表示灰度版本号
	GrayPercent   int    // GrayPercent 表示灰度流量百分比
	GraySalt      string // GraySalt 表示灰度哈希盐值
	Status        int    // Status 表示 AppID 总状态：1启用，0停用
	SignStatus    int    // SignStatus 表示签名验签状态：1启用，0停用
	CryptoStatus  int    // CryptoStatus 表示加密解密状态：1启用，0停用
}

// SignEnabled 返回当前 AppID 是否启用签名验签链路。
func (c *SecretKeyRouteConfig) SignEnabled() bool {
	return c != nil && c.Status == 1 && c.SignStatus == 1
}

// CryptoEnabled 返回当前 AppID 是否启用加密解密链路。
func (c *SecretKeyRouteConfig) CryptoEnabled() bool {
	return c != nil && c.Status == 1 && c.CryptoStatus == 1
}

// SecretKeyLogic 承载签名与加密所需的配置文件秘钥读取逻辑。
type SecretKeyLogic struct {
	*BaseLogic // 复用上下文和 ServiceContext 访问能力
}

// NewSecretKeyLogic 创建秘钥业务逻辑对象。
func NewSecretKeyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SecretKeyLogic {
	return &SecretKeyLogic{BaseLogic: NewBaseLogicWithContext(ctx, svcCtx)}
}

// SecurityConfigured 判断当前 AppID 是否配置了安全链路秘钥。
func (l *SecretKeyLogic) SecurityConfigured(appID string) bool {
	secretCfg, ok := l.currentConfigSecretKey(appID)
	return ok && !configSecretKeyIsEmpty(secretCfg)
}

// GetRouteConfig 读取指定 AppID 的运行时路由配置。
func (l *SecretKeyLogic) GetRouteConfig(appID string) (*SecretKeyRouteConfig, error) {
	return l.getConfigSecretKeyRoute(appID)
}

// GetAESKey 读取指定 AppID 在当前路由命中的 AES KEY 与 IV。
func (l *SecretKeyLogic) GetAESKey(appID string, versionHint string, grayKey string) (*AESKey, string, error) {
	version, err := l.ResolveSecretKeyVersion(appID, versionHint, grayKey)
	if err != nil {
		return nil, "", errors.Tag(err)
	}
	versionCfg, err := l.configSecretKeyVersion(appID, version)
	if err != nil {
		return nil, "", errors.Tag(err)
	}
	key, err := resolveConfigSecretText(versionCfg.AESKey, versionCfg.AESKeyRef, "AES KEY")
	if err != nil {
		return nil, "", errors.Wrap(err, "AES KEY解析失败")
	}
	iv, err := resolveConfigSecretText(versionCfg.AESIV, versionCfg.AESIVRef, "AES IV")
	if err != nil {
		return nil, "", errors.Wrap(err, "AES IV解析失败")
	}
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, "", errors.Errorf("AES KEY长度必须是16、24或32位")
	}
	if len(iv) != 16 {
		return nil, "", errors.Errorf("AES IV长度必须是16位")
	}
	return &AESKey{Key: key, IV: iv}, version, nil
}

// GetRSAKey 读取指定 AppID 在当前路由命中的 RSA PEM 内容。
func (l *SecretKeyLogic) GetRSAKey(appID string, versionHint string, grayKey string, keyType string) (string, string, error) {
	version, err := l.ResolveSecretKeyVersion(appID, versionHint, grayKey)
	if err != nil {
		return "", "", errors.Tag(err)
	}
	versionCfg, err := l.configSecretKeyVersion(appID, version)
	if err != nil {
		return "", "", errors.Tag(err)
	}
	switch keyType {
	case RSAUserPublicKey:
		text, err := resolveConfigPEMText(versionCfg.RSAPublicKeyUser, versionCfg.RSAPublicKeyUserRef, "用户RSA公钥")
		return text, version, errors.Tag(err)
	case RSAServerPublicKey:
		if strings.TrimSpace(versionCfg.RSAPublicKeyServer) == "" && strings.TrimSpace(versionCfg.RSAPublicKeyServerRef) == "" {
			text, err := deriveConfigServerPublicPEM(versionCfg)
			return text, version, errors.Tag(err)
		}
		text, err := resolveConfigPEMText(versionCfg.RSAPublicKeyServer, versionCfg.RSAPublicKeyServerRef, "服务端RSA公钥")
		return text, version, errors.Tag(err)
	case RSAServerPrivateKey:
		text, err := resolveConfigPEMText(versionCfg.RSAPrivateKeyServer, versionCfg.RSAPrivateKeyServerRef, "服务端RSA私钥")
		return text, version, errors.Tag(err)
	default:
		return "", "", errors.Errorf("RSA秘钥类型不合法: %s", keyType)
	}
}

// ResolveSecretKeyVersion 返回当前请求最终命中的秘钥版本。
func (l *SecretKeyLogic) ResolveSecretKeyVersion(appID string, versionHint string, grayKey string) (string, error) {
	route, err := l.getConfigSecretKeyRoute(appID)
	if err != nil {
		return "", errors.Tag(err)
	}
	versionHint = strings.TrimSpace(versionHint)
	if versionHint != "" {
		return versionHint, nil
	}
	if route.GrayVersion != "" && route.GrayPercent > 0 && route.GrayPercent < 100 {
		if hitGray(grayKey, route.GraySalt, route.GrayPercent) {
			return route.GrayVersion, nil
		}
	}
	if route.GrayVersion != "" && route.GrayPercent >= 100 {
		return route.GrayVersion, nil
	}
	return route.StableVersion, nil
}

// currentConfigSecretKey 读取当前运行期配置中的站点秘钥配置，并完成 AppID 匹配。
func (l *SecretKeyLogic) currentConfigSecretKey(appID string) (config.SecuritySecretKeyConfig, bool) {
	if l == nil || l.svc == nil {
		return config.SecuritySecretKeyConfig{}, false
	}
	appID = strings.TrimSpace(appID)
	cfg := l.svc.CurrentConfig()
	configAppID := strings.TrimSpace(cfg.AppID)
	if appID == "" || configAppID == "" || appID != configAppID {
		return config.SecuritySecretKeyConfig{}, false
	}
	return cfg.Security.SecretKey, true
}

// getConfigSecretKeyRoute 从配置文件读取当前 AppID 的版本选路和链路开关。
func (l *SecretKeyLogic) getConfigSecretKeyRoute(appID string) (*SecretKeyRouteConfig, error) {
	secretCfg, ok := l.currentConfigSecretKey(appID)
	if !ok {
		return nil, errors.Errorf("AppID未命中配置文件秘钥: %s", appID)
	}
	if configSecretKeyIsEmpty(secretCfg) {
		return nil, errors.Errorf("配置文件security.secret_key未配置")
	}
	stableVersion := configSecretKeyStableVersion(secretCfg)
	if stableVersion == "" {
		return nil, errors.Errorf("配置文件security.secret_key.key_version未配置")
	}
	return &SecretKeyRouteConfig{
		UUID:          strings.TrimSpace(appID),
		StableVersion: stableVersion,
		GrayVersion:   strings.TrimSpace(secretCfg.GrayVersion),
		GrayPercent:   secretCfg.GrayPercent,
		GraySalt:      strings.TrimSpace(secretCfg.GraySalt),
		Status:        1,
		SignStatus:    secretCfg.SignStatus,
		CryptoStatus:  secretCfg.CryptoStatus,
	}, nil
}

// configSecretKeyVersion 按版本号读取配置文件中的秘钥版本材料。
func (l *SecretKeyLogic) configSecretKeyVersion(appID string, keyVersion string) (config.SecuritySecretKeyVersionConfig, error) {
	secretCfg, ok := l.currentConfigSecretKey(appID)
	if !ok {
		return config.SecuritySecretKeyVersionConfig{}, errors.Errorf("AppID未命中配置文件秘钥: %s", appID)
	}
	keyVersion = strings.TrimSpace(keyVersion)
	if keyVersion == "" {
		return config.SecuritySecretKeyVersionConfig{}, errors.Errorf("秘钥版本不能为空")
	}
	singleVersion := buildSingleConfigSecretKeyVersion(secretCfg)
	for _, item := range secretCfg.Versions {
		if strings.TrimSpace(item.KeyVersion) == keyVersion {
			item.KeyVersion = strings.TrimSpace(item.KeyVersion)
			return item, nil
		}
	}
	if strings.TrimSpace(singleVersion.KeyVersion) == keyVersion {
		return singleVersion, nil
	}
	return config.SecuritySecretKeyVersionConfig{}, errors.Errorf("配置文件security.secret_key.versions未找到版本: %s", keyVersion)
}

// configSecretKeyIsEmpty 判断配置文件秘钥段是否完全未填写。
func configSecretKeyIsEmpty(secretCfg config.SecuritySecretKeyConfig) bool {
	return strings.TrimSpace(secretCfg.KeyVersion) == "" &&
		strings.TrimSpace(secretCfg.AESKey) == "" &&
		strings.TrimSpace(secretCfg.AESKeyRef) == "" &&
		strings.TrimSpace(secretCfg.AESIV) == "" &&
		strings.TrimSpace(secretCfg.AESIVRef) == "" &&
		strings.TrimSpace(secretCfg.RSAPublicKeyUser) == "" &&
		strings.TrimSpace(secretCfg.RSAPublicKeyUserRef) == "" &&
		strings.TrimSpace(secretCfg.RSAPublicKeyServer) == "" &&
		strings.TrimSpace(secretCfg.RSAPublicKeyServerRef) == "" &&
		strings.TrimSpace(secretCfg.RSAPrivateKeyServer) == "" &&
		strings.TrimSpace(secretCfg.RSAPrivateKeyServerRef) == "" &&
		len(secretCfg.Versions) == 0
}

// configSecretKeyStableVersion 返回配置文件秘钥的稳定版本。
func configSecretKeyStableVersion(secretCfg config.SecuritySecretKeyConfig) string {
	stableVersion := strings.TrimSpace(secretCfg.StableVersion)
	if stableVersion != "" {
		return stableVersion
	}
	return strings.TrimSpace(secretCfg.KeyVersion)
}

// buildSingleConfigSecretKeyVersion 把顶层单版本配置转换成统一版本结构。
func buildSingleConfigSecretKeyVersion(secretCfg config.SecuritySecretKeyConfig) config.SecuritySecretKeyVersionConfig {
	return config.SecuritySecretKeyVersionConfig{
		KeyVersion:             configSecretKeyStableVersion(secretCfg),
		AESKey:                 secretCfg.AESKey,
		AESKeyRef:              secretCfg.AESKeyRef,
		AESIV:                  secretCfg.AESIV,
		AESIVRef:               secretCfg.AESIVRef,
		RSAPublicKeyUser:       secretCfg.RSAPublicKeyUser,
		RSAPublicKeyUserRef:    secretCfg.RSAPublicKeyUserRef,
		RSAPublicKeyServer:     secretCfg.RSAPublicKeyServer,
		RSAPublicKeyServerRef:  secretCfg.RSAPublicKeyServerRef,
		RSAPrivateKeyServer:    secretCfg.RSAPrivateKeyServer,
		RSAPrivateKeyServerRef: secretCfg.RSAPrivateKeyServerRef,
	}
}

// resolveConfigSecretText 从配置文件明文或文件引用读取普通秘钥文本。
func resolveConfigSecretText(value string, ref string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		return value, nil
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.Errorf("配置文件%s未配置", label)
	}
	return readSecretFileText(ref)
}

// resolveConfigPEMText 从配置文件明文或文件引用读取 RSA PEM 文本。
func resolveConfigPEMText(value string, ref string, label string) (string, error) {
	text, err := resolveConfigSecretText(value, ref, label)
	if err != nil {
		return "", errors.Tag(err)
	}
	if !strings.Contains(text, "-----BEGIN") {
		return "", errors.Errorf("配置文件%s不是有效PEM", label)
	}
	return text, nil
}

// deriveConfigServerPublicPEM 从配置文件中的服务端私钥派生公钥 PEM。
func deriveConfigServerPublicPEM(versionCfg config.SecuritySecretKeyVersionConfig) (string, error) {
	privatePEM, err := resolveConfigPEMText(versionCfg.RSAPrivateKeyServer, versionCfg.RSAPrivateKeyServerRef, "服务端RSA私钥")
	if err != nil {
		return "", errors.Tag(err)
	}
	privateKey, err := security.ParseRSAPrivateKey(privatePEM)
	if err != nil {
		return "", errors.Wrap(err, "服务端RSA私钥格式不合法")
	}
	return deriveRSAPublicPEMFromPrivateKey(privateKey)
}

// deriveRSAPublicPEMFromPrivateKey 从 RSA 私钥派生公钥 PEM。
func deriveRSAPublicPEMFromPrivateKey(privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", errors.Errorf("服务端RSA私钥为空")
	}
	publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", errors.Tag(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicBytes})), nil
}

// readSecretFileText 读取秘钥文件并去除首尾空白。
func readSecretFileText(path string) (string, error) {
	body, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", errors.Tag(err)
	}
	return strings.TrimSpace(string(body)), nil
}

// hitGray 根据灰度键、盐值和百分比判断是否命中灰度版本。
func hitGray(grayKey string, salt string, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	text := strings.TrimSpace(grayKey)
	if text == "" {
		text = "default"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(salt) + ":" + text))
	return int(h.Sum32()%100) < percent
}
