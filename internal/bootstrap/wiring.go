package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/conf"
)

// LoadConfig 读取并解析配置文件。
func LoadConfig(file string) (config.Config, string, error) {
	c, err := loadBaseConfig(file)
	if err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	if err = applyExternalConfigFiles(file, &c); err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	normalizeConfig(&c)
	if err = validateConfig(c); err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	version, err := configVersion(file)
	if err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	return c, version, nil
}

// loadBaseConfig 只读取主配置文件，不处理 config_files 引用。
func loadBaseConfig(file string) (config.Config, error) {
	var c config.Config
	if err := conf.Load(file, &c); err != nil {
		return config.Config{}, errors.Tag(err)
	}
	return c, nil
}

// Wire 作为应用装配入口，统一负责读取配置并构建 App。
func Wire(ctx context.Context, configFile string) (*App, error) {
	cfg, version, err := LoadConfig(configFile)
	if err != nil {
		return nil, errors.Tag(err)
	}
	app, err := New(ctx, cfg, version)
	if err != nil {
		return nil, errors.Tag(err)
	}
	app.ConfigFile = strings.TrimSpace(configFile)
	return app, nil
}

// normalizeConfig 补齐运行默认值，避免启动期依赖拿到空参数。
func normalizeConfig(c *config.Config) {
	if c == nil {
		return
	}
	if c.Name == "" {
		c.Name = "api"
	}
	if c.Observability.ServiceName == "" {
		c.Observability.ServiceName = c.Name
	}
	if c.Observability.Environment == "" {
		c.Observability.Environment = c.Mode
	}
	if c.JwtExpiresIn <= 0 {
		c.JwtExpiresIn = 86400
	}
	if c.Auth.Issuer == "" {
		c.Auth.Issuer = c.Name
	}
	if c.Auth.SessionTTLSeconds <= 0 {
		c.Auth.SessionTTLSeconds = c.JwtExpiresIn
	}
	if c.Auth.ProfileCacheTTLSeconds <= 0 {
		c.Auth.ProfileCacheTTLSeconds = 300
	}
	if c.Auth.PasswordMinLength <= 0 {
		c.Auth.PasswordMinLength = 8
	}
}

// configVersion 计算配置文件指纹，用于健康检查展示当前配置版本。
func configVersion(file string) (string, error) {
	fingerprint, err := configBundleFingerprint(file)
	if err != nil {
		return "", errors.Tag(err)
	}
	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:8]), nil
}
