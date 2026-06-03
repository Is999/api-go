package config

import (
	"os"
	"testing"

	"api/common/runtimecfg"
	appconfig "api/internal/config"
)

// TestMain 验证对应场景符合预期。
func TestMain(m *testing.M) {
	runtimecfg.Set(appconfig.Config{AppID: "site-a"})
	os.Exit(m.Run())
}
