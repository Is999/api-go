package runtimefile

import (
	"strings"

	"api/internal/config"

	"github.com/Is999/go-utils/errors"
	"github.com/zeromicro/go-zero/core/conf"
)

// Apply 按主配置 config_files 声明合并外部运行期配置。
func Apply(mainFile string, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.ConfigFiles.Runtime) == "" {
		return nil
	}
	path := resolveIncludePath(mainFile, cfg.ConfigFiles.Runtime)
	return errors.Tag(apply(path, cfg))
}

// apply 合并单个外部运行期配置文件。
func apply(path string, cfg *config.Config) error {
	content, keys, err := knownContent(path)
	if err != nil {
		return errors.Tag(err)
	}
	var ext file
	if err = conf.LoadFromYamlBytes(content, &ext); err != nil {
		return errors.Wrapf(err, "加载运行期外部配置失败 file=%s", path)
	}
	for _, spec := range sectionSpecs() {
		if _, ok := keys[spec.Key]; !ok {
			continue
		}
		spec.apply(cfg, ext)
	}
	return nil
}
