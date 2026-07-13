package configload

import (
	"strings"

	"api/internal/bootstrap/configload/runtimefile"
	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/conf"
)

// Load 读取并解析配置文件，供启动期和热加载复用。
func Load(file string) (config.Config, error) {
	c, err := loadBaseConfig(file)
	if err != nil {
		return config.Config{}, errors.Tag(err)
	}
	if err = runtimefile.Apply(file, &c); err != nil {
		return config.Config{}, errors.Tag(err)
	}
	Normalize(&c)
	if err = Validate(c); err != nil {
		return config.Config{}, errors.Tag(err)
	}
	return c, nil
}

// Normalize 补齐运行默认值，避免启动期依赖拿到空参数。
func Normalize(c *config.Config) {
	if c == nil {
		return
	}
	if c.Name == "" {
		c.Name = "api"
	}
	if c.Observability.ServiceName == "" {
		c.Observability.ServiceName = c.Name
	}
	c.Observability.Environment = strings.TrimSpace(c.Mode)
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
	if c.User.RouteShardCount == 0 {
		c.User.RouteShardCount = defaultUserRouteShardCount
	}
}

// loadBaseConfig 只读取主配置文件，不处理 config_files 引用。
func loadBaseConfig(file string) (config.Config, error) {
	var c config.Config
	if err := conf.Load(file, &c); err != nil {
		return config.Config{}, errors.Tag(err)
	}
	return c, nil
}
