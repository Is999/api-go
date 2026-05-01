package auth

import (
	"os"
	"testing"

	"api/common/runtimecfg"
	"api/internal/config"
)

func TestMain(m *testing.M) {
	runtimecfg.Set(config.Config{AppID: "site-a"})
	os.Exit(m.Run())
}
