package luaplugin

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

var pomodoroMinutesLeft = regexp.MustCompile(`(\d+)m \d+s left`)

// waitMinutesLeft polls the status command until want(minutes) holds, since key
// handlers may land after EmitKey returns.
func waitMinutesLeft(t *testing.T, mgr *Manager, want func(int) bool) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		out, _ := mgr.EmitCommand("pomodoro", "status", nil)
		if m := pomodoroMinutesLeft.FindStringSubmatch(out); m != nil {
			n, _ := strconv.Atoi(m[1])
			if want(n) || time.Now().After(deadline) {
				return n
			}
		} else if time.Now().After(deadline) {
			t.Fatalf("status = %q, want a running phase", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestPomodoroKeys covers the H / ( / ) bindings, with F reserved the way
// upstream's subscribed-shows overlay now reserves it.
func TestPomodoroKeys(t *testing.T) {
	SetDefaultReservedKeys(map[string]bool{"F": true})
	t.Cleanup(func() { SetDefaultReservedKeys(nil) })

	mgr := loadStudyPlugins(t, map[string]map[string]string{
		"pomodoro": {"work_minutes": "25", "adjust_minutes": "5"},
	}, "pomodoro")

	if !mgr.EmitKey("H") {
		t.Fatal(`EmitKey("H") did not reach the pomodoro`)
	}
	if got := waitMinutesLeft(t, mgr, func(n int) bool { return n == 24 }); got != 24 {
		t.Fatalf("after H: %d min left, want 24 (a fresh 25 min phase)", got)
	}

	mgr.EmitKey(")")
	if got := waitMinutesLeft(t, mgr, func(n int) bool { return n == 29 }); got != 29 {
		t.Fatalf("after ): %d min left, want 29", got)
	}

	mgr.EmitKey("(")
	mgr.EmitKey("(")
	if got := waitMinutesLeft(t, mgr, func(n int) bool { return n == 19 }); got != 19 {
		t.Fatalf("after ( twice: %d min left, want 19", got)
	}

	// Shortening never goes below one minute.
	for range 10 {
		mgr.EmitKey("(")
	}
	if got := waitMinutesLeft(t, mgr, func(n int) bool { return n == 1 }); got != 1 {
		t.Fatalf("after many (: %d min left, want the 1 min floor", got)
	}

	if mgr.EmitKey("F") {
		t.Error(`"F" reached a plugin binding although core reserves it`)
	}
}
