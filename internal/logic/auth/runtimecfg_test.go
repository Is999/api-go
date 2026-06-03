package auth

import (
	"os"
	"testing"

	"api/common/runtimecfg"
	"api/internal/config"
)

// TestMain 验证对应场景符合预期。
func TestMain(m *testing.M) {
	runtimecfg.Set(config.Config{AppID: "site-a"})
	os.Exit(m.Run())
}
