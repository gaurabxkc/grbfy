package ipc

import (
	"os"
	"path/filepath"

	"github.com/bjarneo/cliamp/internal/appdir"
)

// DefaultSocketPath returns the default IPC socket path (~/.config/grbfy/grbfy.sock).
func DefaultSocketPath() string {
	dir, err := appdir.Dir()
	if err != nil {
		return filepath.Join(os.TempDir(), "grbfy.sock")
	}
	return filepath.Join(dir, "grbfy.sock")
}
