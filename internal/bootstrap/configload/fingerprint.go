package configload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"api/internal/bootstrap/configload/runtimefile"

	utils "github.com/Is999/go-utils"
	"github.com/Is999/go-utils/errors"
)

// configFileFingerprint 返回单个配置文件当前的稳定指纹。
func configFileFingerprint(file string) (string, error) {
	cleanFile := filepath.Clean(strings.TrimSpace(file))
	info, err := os.Stat(cleanFile)
	if err != nil {
		return "", errors.Tag(err)
	}
	data, err := os.ReadFile(cleanFile)
	if err != nil {
		return "", errors.Tag(err)
	}
	realPath, err := filepath.EvalSymlinks(cleanFile)
	if err != nil {
		realPath = cleanFile
	}
	return fmt.Sprintf("%s|%d|%d|%s", realPath, info.Size(), info.ModTime().UnixNano(), utils.SHA256(string(data))), nil
}

// BundleFingerprint 返回主配置及外部配置文件组成的配置包指纹。
func BundleFingerprint(file string) (string, error) {
	mainFingerprint, err := configFileFingerprint(file)
	if err != nil {
		return "", errors.Tag(err)
	}
	cfg, err := loadBaseConfig(file)
	if err != nil {
		return mainFingerprint, nil
	}
	parts := []string{mainFingerprint}
	for _, include := range runtimefile.IncludePaths(file, cfg.ConfigFiles) {
		fingerprint, innerErr := configFileFingerprint(include)
		if innerErr != nil {
			return "", errors.Wrapf(innerErr, "读取外部配置文件指纹失败 file=%s", include)
		}
		parts = append(parts, fingerprint)
	}
	return strings.Join(parts, "\n"), nil
}

// Version 计算配置文件指纹短版本，用于健康检查展示当前配置版本。
func Version(file string) (string, error) {
	fingerprint, err := BundleFingerprint(file)
	if err != nil {
		return "", errors.Tag(err)
	}
	sum := sha256.Sum256([]byte(fingerprint))
	return hex.EncodeToString(sum[:8]), nil
}
