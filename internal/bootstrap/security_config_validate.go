package bootstrap

import (
	"os"
	"strconv"
	"strings"

	"api/internal/config"
	"api/internal/security"

	"github.com/Is999/go-utils/errors"
)

// 安全配置校验边界常量。
const (
	maxSecurityKeyVersionLength = 64 // 秘钥版本号最大长度，保持请求头和配置一致
)

// securitySecretKeyVersionItem 绑定秘钥版本配置和来源路径，便于精准报错。
type securitySecretKeyVersionItem struct {
	source string                                // source 表示配置来源路径，便于启动报错定位
	value  config.SecuritySecretKeyVersionConfig // value 表示待校验的单版本秘钥材料
}

// validateSecurityConfig 校验前台签名验签和加解密配置的版本路由与材料。
func validateSecurityConfig(c config.Config) error {
	secretCfg := c.Security.SecretKey
	if configSecuritySecretKeyIsEmpty(secretCfg) {
		return nil
	}
	if strings.TrimSpace(c.AppID) == "" {
		return errors.Errorf("security.secret_key 已配置时 app_id 不能为空")
	}
	if err := validateSecuritySecretKeyRoute(secretCfg, isProductionMode(c.Mode)); err != nil {
		return errors.Tag(err)
	}
	return nil
}

// validateSecuritySecretKeyRoute 校验稳定版本、灰度版本和链路开关是否自洽。
func validateSecuritySecretKeyRoute(secretCfg config.SecuritySecretKeyConfig, production bool) error {
	if err := validateSecuritySwitch("security.secret_key.sign_status", secretCfg.SignStatus); err != nil {
		return errors.Tag(err)
	}
	if err := validateSecuritySwitch("security.secret_key.crypto_status", secretCfg.CryptoStatus); err != nil {
		return errors.Tag(err)
	}
	if secretCfg.GrayPercent < 0 || secretCfg.GrayPercent > 100 {
		return errors.Errorf("security.secret_key.gray_percent 必须在 0-100 之间")
	}

	stableVersion := configSecurityStableVersion(secretCfg)
	if stableVersion == "" {
		return errors.Errorf("security.secret_key.key_version 或 stable_version 不能为空")
	}
	if err := validateSecurityKeyVersion("security.secret_key.stable_version", stableVersion); err != nil {
		return errors.Tag(err)
	}

	versionItems, err := configSecurityVersionItems(secretCfg)
	if err != nil {
		return errors.Tag(err)
	}
	versionByName := make(map[string]securitySecretKeyVersionItem, len(versionItems))
	for _, item := range versionItems {
		version := strings.TrimSpace(item.value.KeyVersion)
		if err := validateSecurityKeyVersion(item.source+".key_version", version); err != nil {
			return errors.Tag(err)
		}
		if _, exists := versionByName[version]; exists {
			return errors.Errorf("security.secret_key 版本[%s]重复配置", version)
		}
		versionByName[version] = item
	}
	if _, ok := versionByName[stableVersion]; !ok {
		return errors.Errorf("security.secret_key 稳定版本[%s]不存在", stableVersion)
	}

	grayVersion := strings.TrimSpace(secretCfg.GrayVersion)
	if grayVersion != "" {
		if err := validateSecurityKeyVersion("security.secret_key.gray_version", grayVersion); err != nil {
			return errors.Tag(err)
		}
		if _, ok := versionByName[grayVersion]; !ok {
			return errors.Errorf("security.secret_key 灰度版本[%s]不存在", grayVersion)
		}
	}
	if secretCfg.GrayPercent > 0 {
		if grayVersion == "" {
			return errors.Errorf("security.secret_key.gray_percent 大于 0 时必须配置 gray_version")
		}
		if grayVersion == stableVersion {
			return errors.Errorf("security.secret_key.gray_version 不能与稳定版本相同")
		}
		if production && strings.TrimSpace(secretCfg.GraySalt) == "" {
			return errors.Errorf("生产环境启用秘钥灰度时必须配置 security.secret_key.gray_salt")
		}
	}

	signEnabled := secretCfg.SignStatus == 1
	cryptoEnabled := secretCfg.CryptoStatus == 1
	for _, item := range versionItems {
		if err := validateSecuritySecretKeyVersion(item, signEnabled, cryptoEnabled, production); err != nil {
			return errors.Tag(err)
		}
	}
	return nil
}

// validateSecuritySwitch 校验安全链路开关只能使用 0 或 1。
func validateSecuritySwitch(name string, value int) error {
	if value != 0 && value != 1 {
		return errors.Errorf("%s 只能是 0 或 1", name)
	}
	return nil
}

// validateSecurityKeyVersion 校验秘钥版本号格式，避免请求头选路出现歧义。
func validateSecurityKeyVersion(name string, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.Errorf("%s 不能为空", name)
	}
	if len(version) > maxSecurityKeyVersionLength {
		return errors.Errorf("%s 长度不能超过 %d", name, maxSecurityKeyVersionLength)
	}
	if strings.ContainsAny(version, " \t\r\n") || strings.ContainsAny(version, "*?") {
		return errors.Errorf("%s 不能包含空白或通配符", name)
	}
	return nil
}

// validateSecuritySecretKeyVersion 校验单个版本在当前链路开关下所需的材料。
func validateSecuritySecretKeyVersion(item securitySecretKeyVersionItem, signEnabled bool, cryptoEnabled bool, production bool) error {
	versionCfg := item.value
	if cryptoEnabled {
		key, err := resolveSecuritySecretText(versionCfg.AESKey, versionCfg.AESKeyRef, item.source+".aes_key", production)
		if err != nil {
			return errors.Wrap(err, "AES KEY不可用")
		}
		iv, err := resolveSecuritySecretText(versionCfg.AESIV, versionCfg.AESIVRef, item.source+".aes_iv", production)
		if err != nil {
			return errors.Wrap(err, "AES IV不可用")
		}
		if len(key) != 16 && len(key) != 24 && len(key) != 32 {
			return errors.Errorf("%s AES KEY长度必须是16、24或32位", item.source)
		}
		if len(iv) != 16 {
			return errors.Errorf("%s AES IV长度必须是16位", item.source)
		}
		if _, err := security.NewAESCipher(key, iv); err != nil {
			return errors.Wrap(err, "AES加解密器初始化失败")
		}
	}
	if signEnabled || cryptoEnabled {
		if _, err := resolveSecurityRSAPublicKey(versionCfg.RSAPublicKeyUser, versionCfg.RSAPublicKeyUserRef, item.source+".rsa_public_key_user", production); err != nil {
			return errors.Wrap(err, "用户RSA公钥不可用")
		}
		serverPrivatePEM, err := resolveSecurityRSAPrivateKey(versionCfg.RSAPrivateKeyServer, versionCfg.RSAPrivateKeyServerRef, item.source+".rsa_private_key_server", production)
		if err != nil {
			return errors.Wrap(err, "服务端RSA私钥不可用")
		}
		if hasSecuritySecretValue(versionCfg.RSAPublicKeyServer, versionCfg.RSAPublicKeyServerRef) {
			if _, err := resolveSecurityRSAPublicKey(versionCfg.RSAPublicKeyServer, versionCfg.RSAPublicKeyServerRef, item.source+".rsa_public_key_server", production); err != nil {
				return errors.Wrap(err, "服务端RSA公钥不可用")
			}
		} else if signEnabled {
			if _, err := security.ParseRSAPrivateKey(serverPrivatePEM); err != nil {
				return errors.Wrap(err, "服务端RSA公钥派生失败")
			}
		}
	}
	return nil
}

// resolveSecuritySecretText 读取明文或文件引用秘钥，禁止同一字段双来源配置。
func resolveSecuritySecretText(value string, ref string, name string, production bool) (string, error) {
	value = strings.TrimSpace(value)
	ref = strings.TrimSpace(ref)
	if value != "" && ref != "" {
		return "", errors.Errorf("%s 只能配置明文或文件引用之一", name)
	}
	if value != "" {
		if production && isPlaceholderSecret(value) {
			return "", errors.Errorf("生产环境 %s 不能使用占位值", name)
		}
		return value, nil
	}
	if ref == "" {
		return "", errors.Errorf("%s 未配置", name)
	}
	body, err := os.ReadFile(ref)
	if err != nil {
		return "", errors.Wrapf(err, "读取 %s 文件失败", name)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", errors.Errorf("%s 文件内容为空", name)
	}
	if production && isPlaceholderSecret(text) {
		return "", errors.Errorf("生产环境 %s 不能使用占位值", name)
	}
	return text, nil
}

// resolveSecurityRSAPublicKey 读取并校验 RSA 公钥 PEM。
func resolveSecurityRSAPublicKey(value string, ref string, name string, production bool) (string, error) {
	text, err := resolveSecuritySecretText(value, ref, name, production)
	if err != nil {
		return "", errors.Tag(err)
	}
	if !strings.Contains(text, "-----BEGIN") {
		return "", errors.Errorf("%s 不是有效PEM", name)
	}
	if _, err := security.NewRSASigner("", text); err != nil {
		return "", errors.Tag(err)
	}
	return text, nil
}

// resolveSecurityRSAPrivateKey 读取并校验 RSA 私钥 PEM。
func resolveSecurityRSAPrivateKey(value string, ref string, name string, production bool) (string, error) {
	text, err := resolveSecuritySecretText(value, ref, name, production)
	if err != nil {
		return "", errors.Tag(err)
	}
	if !strings.Contains(text, "-----BEGIN") {
		return "", errors.Errorf("%s 不是有效PEM", name)
	}
	if _, err := security.NewRSASigner(text, ""); err != nil {
		return "", errors.Tag(err)
	}
	return text, nil
}

// configSecurityVersionItems 收敛单版本和多版本配置，保持运行期版本选路语义。
func configSecurityVersionItems(secretCfg config.SecuritySecretKeyConfig) ([]securitySecretKeyVersionItem, error) {
	items := make([]securitySecretKeyVersionItem, 0, len(secretCfg.Versions)+1)
	for index, versionCfg := range secretCfg.Versions {
		versionCfg.KeyVersion = strings.TrimSpace(versionCfg.KeyVersion)
		items = append(items, securitySecretKeyVersionItem{
			source: "security.secret_key.versions[" + strconv.Itoa(index) + "]",
			value:  versionCfg,
		})
	}
	if configSecurityTopVersionHasMaterial(secretCfg) || len(secretCfg.Versions) == 0 {
		singleVersion := buildConfigSecuritySecretKeyVersion(secretCfg)
		items = append(items, securitySecretKeyVersionItem{
			source: "security.secret_key",
			value:  singleVersion,
		})
	}
	if len(items) == 0 {
		return nil, errors.Errorf("security.secret_key 至少需要一个秘钥版本")
	}
	return items, nil
}

// configSecuritySecretKeyIsEmpty 判断配置文件秘钥段是否完全未填写。
func configSecuritySecretKeyIsEmpty(secretCfg config.SecuritySecretKeyConfig) bool {
	return strings.TrimSpace(secretCfg.KeyVersion) == "" &&
		!configSecurityTopVersionHasMaterial(secretCfg) &&
		len(secretCfg.Versions) == 0
}

// configSecurityTopVersionHasMaterial 判断顶层单版本配置是否包含真实材料字段。
func configSecurityTopVersionHasMaterial(secretCfg config.SecuritySecretKeyConfig) bool {
	return hasSecuritySecretValue(secretCfg.AESKey, secretCfg.AESKeyRef) ||
		hasSecuritySecretValue(secretCfg.AESIV, secretCfg.AESIVRef) ||
		hasSecuritySecretValue(secretCfg.RSAPublicKeyUser, secretCfg.RSAPublicKeyUserRef) ||
		hasSecuritySecretValue(secretCfg.RSAPublicKeyServer, secretCfg.RSAPublicKeyServerRef) ||
		hasSecuritySecretValue(secretCfg.RSAPrivateKeyServer, secretCfg.RSAPrivateKeyServerRef)
}

// hasSecuritySecretValue 判断一个秘钥字段是否配置了明文或文件引用。
func hasSecuritySecretValue(value string, ref string) bool {
	return strings.TrimSpace(value) != "" || strings.TrimSpace(ref) != ""
}

// configSecurityStableVersion 返回配置文件秘钥的稳定版本。
func configSecurityStableVersion(secretCfg config.SecuritySecretKeyConfig) string {
	stableVersion := strings.TrimSpace(secretCfg.StableVersion)
	if stableVersion != "" {
		return stableVersion
	}
	return strings.TrimSpace(secretCfg.KeyVersion)
}

// buildConfigSecuritySecretKeyVersion 把顶层单版本配置转换成统一版本结构。
func buildConfigSecuritySecretKeyVersion(secretCfg config.SecuritySecretKeyConfig) config.SecuritySecretKeyVersionConfig {
	return config.SecuritySecretKeyVersionConfig{
		KeyVersion:             configSecurityStableVersion(secretCfg),
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
