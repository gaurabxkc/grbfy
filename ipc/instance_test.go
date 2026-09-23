package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunningInstanceSeesALivePlayer(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "grbfy.sock")
	ln, err := listenSocket(sock)
	if err != nil {
		t.Fatalf("listening on the test socket: %v", err)
	}
	defer ln.Close()
	if err := os.WriteFile(sock+".pid", []byte("4242\n"), 0o600); err != nil {
		t.Fatalf("writing the pid file: %v", err)
	}

	pid, running := RunningInstance(sock)
	if !running {
		t.Fatal("a listening socket was not seen as a running player")
	}
	if pid != 4242 {
		t.Fatalf("pid = %d, want 4242", pid)
	}
}

// A socket file left behind by a crash has no listener, and must not stop a
// new player from starting.
func TestRunningInstanceIgnoresAStaleSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "grbfy.sock")
	ln, err := listenSocket(sock)
	if err != nil {
		t.Fatalf("listening on the test socket: %v", err)
	}
	// Closing a unix listener normally removes the file; recreate it so it
	// is really left behind, like after a crash.
	_ = ln.Close()
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatalf("leaving a stale socket file: %v", err)
	}

	if _, running := RunningInstance(sock); running {
		t.Fatal("a stale socket file was taken for a running player")
	}
}

func TestRunningInstanceWithNoSocket(t *testing.T) {
	if _, running := RunningInstance(filepath.Join(t.TempDir(), "absent.sock")); running {
		t.Fatal("no socket at all was taken for a running player")
	}
}
