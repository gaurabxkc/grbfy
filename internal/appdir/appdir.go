package appdir

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the grbfy configuration directory.
//
// Resolution order:
//   - GRBFY_CONFIG_DIR (explicit override)
//   - CLIAMP_CONFIG_DIR (upstream's name for the same override)
//   - XDG_CONFIG_HOME/grbfy
//   - HOME/.config/grbfy
//   - on Windows: APPDATA/grbfy
//   - fallback: os.UserHomeDir()/.config/grbfy
//
// CLIAMP_CONFIG_DIR is honored because upstream's tests isolate themselves
// with it. Without it, every upstream test added after the fork writes into
// the real ~/.config/grbfy, which has already clobbered live config once.
func Dir() (string, error) {
	for _, env := range []string{"GRBFY_CONFIG_DIR", "CLIAMP_CONFIG_DIR"} {
		if dir, ok := os.LookupEnv(env); ok && dir != "" {
			return dir, nil
		}
	}
	if xdg, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, "grbfy"), nil
	}
	if home, ok := os.LookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".config", "grbfy"), nil
	}
	if runtime.GOOS == "windows" {
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "grbfy"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "grbfy"), nil
}

// PluginDir returns the grbfy plugin directory.
func PluginDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plugins"), nil
}

// DataDir returns the grbfy data directory (~/.local/share/grbfy), used for
// state that is not user-edited config: plugin stores, downloaded assets, etc.
func DataDir() (string, error) {
	// Honor HOME first, matching Dir(); on Windows os.UserHomeDir() reads
	// USERPROFILE and ignores HOME, so this keeps the two resolvers consistent.
	if home, ok := os.LookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".local", "share", "grbfy"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "grbfy"), nil
}
