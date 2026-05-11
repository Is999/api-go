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

// 运行期外部配置支持的顶层配置段。
const (
	runtimeConfigSectionAuth      = "auth"       // 前台认证运行参数
	runtimeConfigSectionHotReload = "hot_reload" // 配置热加载运行参数
	runtimeConfigSectionSecurity  = "security"   // 签名验签和加解密配置
	runtimeConfigSectionCollector = "collector"  // 通用收集器配置
	runtimeConfigSectionOps       = "ops"        // 运维级接口保护配置
)

// runtimeConfigFile 描述外部运行期配置文件。
type runtimeConfigFile struct {
	Auth      config.AuthConfig      `json:"auth,optional"`       // 前台认证运行参数
	HotReload config.HotReloadConfig `json:"hot_reload,optional"` // 配置热加载运行参数
	Security  config.SecurityConfig  `json:"security,optional"`   // 签名验签和加解密配置
	Collector config.CollectorConfig `json:"collector,optional"`  // 通用收集器配置
	Ops       config.OpsConfig       `json:"ops,optional"`        // 运维级接口保护配置
}

// runtimeConfigSectionSpec 描述一个允许运行期外置的配置段。
type runtimeConfigSectionSpec struct {
	Key   string                                          // 外部运行期配置文件中的顶层键
	apply func(cfg *config.Config, ext runtimeConfigFile) // 将该配置段合并到主配置
}

// runtimeConfigSectionSpecs 返回运行期外部配置段规格。
func runtimeConfigSectionSpecs() []runtimeConfigSectionSpec {
	return []runtimeConfigSectionSpec{
		{
			Key: runtimeConfigSectionAuth,
			apply: func(cfg *config.Config, ext runtimeConfigFile) {
				cfg.Auth = ext.Auth
			},
		},
		{
			Key: runtimeConfigSectionHotReload,
			apply: func(cfg *config.Config, ext runtimeConfigFile) {
				cfg.HotReload = ext.HotReload
			},
		},
		{
			Key: runtimeConfigSectionSecurity,
			apply: func(cfg *config.Config, ext runtimeConfigFile) {
				cfg.Security = ext.Security
			},
		},
		{
			Key: runtimeConfigSectionCollector,
			apply: func(cfg *config.Config, ext runtimeConfigFile) {
				cfg.Collector = ext.Collector
			},
		},
		{
			Key: runtimeConfigSectionOps,
			apply: func(cfg *config.Config, ext runtimeConfigFile) {
				cfg.Ops = ext.Ops
			},
		},
	}
}

// runtimeConfigSectionKeys 返回运行期外部配置段白名单。
func runtimeConfigSectionKeys() map[string]struct{} {
	specs := runtimeConfigSectionSpecs()
	keys := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		keys[spec.Key] = struct{}{}
	}
	return keys
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
	for _, spec := range runtimeConfigSectionSpecs() {
		if _, ok := keys[spec.Key]; !ok {
			continue
		}
		spec.apply(cfg, ext)
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
	knownKeys := runtimeConfigSectionKeys()
	filtered := yaml.MapSlice{}
	for _, item := range root {
		key, ok := item.Key.(string)
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, known := knownKeys[key]; known {
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
