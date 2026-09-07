package luaplugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bjarneo/cliamp/internal/plugintrust"
)

// newManagerWithPlugin loads inline Lua through the real New() path, which is
// where plugins actually bind their keys.
func newManagerWithPlugin(t *testing.T, name, src string) *Manager {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GRBFY_CONFIG_DIR", "")
	dir := filepath.Join(home, ".config", "grbfy", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, name+".lua")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := plugintrust.Approve(dir, name, path); err != nil {
		t.Fatalf("approve: %v", err)
	}

	m, err := New(nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(m.Close)
	return m
}

// Plugins bind during New(), so a reserved key must be refused at load time.
// Before SetDefaultReservedKeys existed the guard was always empty then, and a
// plugin could bind a core key and silently never fire.
func TestDefaultReservedKeysApplyDuringLoad(t *testing.T) {
	t.Cleanup(func() { SetDefaultReservedKeys(nil) })
	SetDefaultReservedKeys(map[string]bool{"x": true})

	m := newManagerWithPlugin(t, "kb", `
		local p = plugin.register({name = "kb", type = "hook", permissions = {"keymap"}})
		local ok, err = p:bind("x", function() end)
		if not ok then grbfy.log.warn("refused: " .. tostring(err)) end
	`)

	if !m.reservedKeys["x"] {
		t.Fatal("Manager from New() did not pick up the default reserved keys")
	}
	if m.EmitKey("x") {
		t.Error("a binding was registered on a reserved key")
	}
}

// An uppercase letter is a different key from its lowercase form, so binding
// it must not be refused just because the lowercase one is reserved. This is
// the exact case that made the pomodoro plugin's F key silently dead.
func TestUppercaseKeyIsNotReservedByLowercase(t *testing.T) {
	t.Cleanup(func() { SetDefaultReservedKeys(nil) })
	SetDefaultReservedKeys(map[string]bool{"f": true})

	m := newManagerWithPlugin(t, "kb", `
		local p = plugin.register({name = "kb", type = "hook", permissions = {"keymap"}})
		p:bind("F", function() end)
	`)

	if !m.EmitKey("F") {
		t.Error(`EmitKey("F") did not reach the binding, with only "f" reserved`)
	}
	if m.EmitKey("f") {
		t.Error(`EmitKey("f") fired a binding registered for "F"`)
	}
}

func TestSetDefaultReservedKeysCopies(t *testing.T) {
	t.Cleanup(func() { SetDefaultReservedKeys(nil) })

	src := map[string]bool{"a": true}
	SetDefaultReservedKeys(src)
	src["b"] = true // must not leak into the stored defaults

	got := initialReservedKeys()
	if got["b"] {
		t.Error("SetDefaultReservedKeys retained the caller's map")
	}
	if !got["a"] {
		t.Error("stored defaults lost an entry")
	}
}
