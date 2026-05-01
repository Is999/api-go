package config

import (
	"os"
	"testing"

	"api/common/runtimecfg"
	appconfig "api/internal/config"
)

func TestMain(m *testing.M) {
	runtimecfg.Set(appconfig.Config{AppID: "site-a"})
	os.Exit(m.Run())
}
