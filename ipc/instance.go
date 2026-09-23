package ipc

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// RunningInstance reports whether another grbfy is already serving the socket,
// and its PID when the PID file names one (0 when it does not).
//
// A player should not start while one is running. The second one cannot claim
// the socket, so it runs without remote control, and its plugins, which talk
// to "grbfy" through `remote call`, silently drive the first instance instead:
// tracks an autoplay plugin queues end up in a window the user is not looking
// at. The probe is a connect, the same one cleanStaleSocket trusts, so a
// socket left behind by a crash is never mistaken for a live player.
func RunningInstance(sockPath string) (pid int, running bool) {
	conn, err := dialSocket(sockPath, 200*time.Millisecond)
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	if data, err := os.ReadFile(sockPath + ".pid"); err == nil {
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	return pid, true
}
