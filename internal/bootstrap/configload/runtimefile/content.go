package runtimefile

import (
	"os"
	"strings"

	"github.com/Is999/go-utils/errors"
	yaml "go.yaml.in/yaml/v2"
)

// knownContent 提取当前版本认识的运行期配置块。
func knownContent(path string) ([]byte, map[string]struct{}, error) {
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
	knownKeys := sectionKeys()
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
