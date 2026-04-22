package bootstrap

import (
	"os"
	"path/filepath"
	"strings"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/conf"
	yaml "go.yaml.in/yaml/v2"
)

// runtimeConfigKnownKeys 定义前台 API 允许从外部运行期配置读取的顶层键。
var runtimeConfigKnownKeys = map[string]struct{}{
	"auth":       {}, // 前台认证运行参数
	"hot_reload": {}, // 配置热加载运行参数
	"security":   {}, // 签名验签和加解密配置
	"collector":  {}, // 通用收集器配置
	"ops":        {}, // 运维级接口保护配置
}

// runtimeConfigFile 描述外部运行期配置文件。
type runtimeConfigFile struct {
	Auth      config.AuthConfig      `json:"auth,optional"`       // 前台认证运行参数
	HotReload config.HotReloadConfig `json:"hot_reload,optional"` // 配置热加载运行参数
	Security  config.SecurityConfig  `json:"security,optional"`   // 签名验签和加解密配置
	Collector config.CollectorConfig `json:"collector,optional"`  // 通用收集器配置
	Ops       config.OpsConfig       `json:"ops,optional"`        // 运维级接口保护配置
}

// applyExternalConfigFiles 按主配置 config_files 声明合并外部运行期配置。
func applyExternalConfigFiles(mainFile string, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.ConfigFiles.Runtime) == "" {
		return nil
	}
	path := resolveConfigIncludePath(mainFile, cfg.ConfigFiles.Runtime)
	return errors.Tag(applyRuntimeConfigFile(path, cfg))
}

// configIncludePaths 返回主配置声明的外部配置文件解析结果。
func configIncludePaths(mainFile string, files config.ConfigFilesConfig) []string {
	if strings.TrimSpace(files.Runtime) == "" {
		return nil
	}
	return []string{resolveConfigIncludePath(mainFile, files.Runtime)}
}

// applyRuntimeConfigFile 合并单个外部运行期配置文件。
func applyRuntimeConfigFile(path string, cfg *config.Config) error {
	content, keys, err := runtimeConfigContent(path)
	if err != nil {
		return errors.Tag(err)
	}
	var ext runtimeConfigFile
	if err = conf.LoadFromYamlBytes(content, &ext); err != nil {
		return errors.Wrapf(err, "加载运行期外部配置失败 file=%s", path)
	}
	if _, ok := keys["auth"]; ok {
		cfg.Auth = ext.Auth
	}
	if _, ok := keys["hot_reload"]; ok {
		cfg.HotReload = ext.HotReload
	}
	if _, ok := keys["security"]; ok {
		cfg.Security = ext.Security
	}
	if _, ok := keys["collector"]; ok {
		cfg.Collector = ext.Collector
	}
	if _, ok := keys["ops"]; ok {
		cfg.Ops = ext.Ops
	}
	return nil
}

// resolveConfigIncludePath 解析外部配置文件路径。
func resolveConfigIncludePath(mainFile string, include string) string {
	include = strings.TrimSpace(include)
	if include == "" || filepath.IsAbs(include) {
		return filepath.Clean(include)
	}
	baseDir := filepath.Dir(filepath.Clean(mainFile))
	return filepath.Clean(filepath.Join(baseDir, include))
}

// runtimeConfigContent 提取当前版本认识的运行期配置块。
func runtimeConfigContent(path string) ([]byte, map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "读取运行期外部配置失败 file=%s", path)
	}
	keys := make(map[string]struct{})
	if len(strings.TrimSpace(string(data))) == 0 {
		return []byte("{}\n"), keys, nil
	}
	var root yaml.MapSlice
	if err = yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, errors.Wrapf(err, "解析运行期外部配置失败 file=%s", path)
	}
	filtered := yaml.MapSlice{}
	for _, item := range root {
		key, ok := item.Key.(string)
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, known := runtimeConfigKnownKeys[key]; known {
			keys[key] = struct{}{}
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return []byte("{}\n"), keys, nil
	}
	content, err := yaml.Marshal(filtered)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "提取运行期外部配置失败 file=%s", path)
	}
	return content, keys, nil
}
