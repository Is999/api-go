package bootstrap

import (
	"context"
	"strings"

	"api/internal/bootstrap/configload"
	"api/internal/config"

	"github.com/Is999/go-utils/errors"
)

// LoadConfig 读取并解析配置文件。
func LoadConfig(file string) (config.Config, string, error) {
	c, err := configload.Load(file)
	if err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	version, err := configload.Version(file)
	if err != nil {
		return config.Config{}, "", errors.Tag(err)
	}
	return c, version, nil
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
