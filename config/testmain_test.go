package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Unsetenv("GRBFY_CONFIG_DIR")
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Exit(m.Run())
}
