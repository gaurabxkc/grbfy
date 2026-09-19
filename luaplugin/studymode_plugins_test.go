package luaplugin

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/internal/plugintrust"
)

// loadStudyPlugins installs the named bundled plugins into an isolated HOME and
// returns a live Manager for them. The bundled-plugin regression test only
// proves these register; this exercises the command and timer paths that only
// run on interaction.
func loadStudyPlugins(t *testing.T, cfg map[string]map[string]string, names ...string) *Manager {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GRBFY_CONFIG_DIR", "")
	t.Setenv("CLIAMP_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "") // appdir checks both before HOME
	pluginDir := filepath.Join(home, ".config", "grbfy", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join("..", "plugins", name+".lua"))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		dest := filepath.Join(pluginDir, name+".lua")
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := plugintrust.Approve(pluginDir, name, dest); err != nil {
			t.Fatalf("approve %s: %v", name, err)
		}
	}

	mgr, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(mgr.Close)
	return mgr
}

func TestSleepTimerCommands(t *testing.T) {
	mgr := loadStudyPlugins(t, nil, "sleep-timer")

	if out, err := mgr.EmitCommand("sleep-timer", "status", nil); err != nil {
		t.Fatalf("status: %v", err)
	} else if !strings.Contains(out, "off") {
		t.Errorf("status before arming = %q, want it to report off", out)
	}

	out, err := mgr.EmitCommand("sleep-timer", "set", []string{"30"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(out, "29m") && !strings.Contains(out, "30m") {
		t.Errorf("set 30 = %q, want roughly 30 minutes left", out)
	}

	if out, err := mgr.EmitCommand("sleep-timer", "cancel", nil); err != nil {
		t.Fatalf("cancel: %v", err)
	} else if !strings.Contains(out, "cancelled") {
		t.Errorf("cancel = %q", out)
	}

	if out, _ := mgr.EmitCommand("sleep-timer", "status", nil); !strings.Contains(out, "off") {
		t.Errorf("status after cancel = %q, want off", out)
	}

	// A bad argument must be reported, not silently ignored.
	if out, _ := mgr.EmitCommand("sleep-timer", "set", []string{"soon"}); !strings.Contains(out, "usage") {
		t.Errorf("set with a non-number = %q, want usage text", out)
	}
}

// The sleep timer must put the volume back after it stops playback, or the
// fade would silently persist into the next session.
func TestSleepTimerFadesThenStopsAndRestoresVolume(t *testing.T) {
	cfg := map[string]map[string]string{
		"sleep-timer": {"fade_seconds": "2"},
	}
	mgr := loadStudyPlugins(t, cfg, "sleep-timer")

	var mu sync.Mutex
	var volumes []float64
	stopped := false

	mgr.SetStateProvider(StateProvider{
		PlayerState: func() string { return "playing" },
		Volume:      func() float64 { return -5 },
	})
	mgr.SetControlProvider(ControlProvider{
		SetVolume: func(db float64) {
			mu.Lock()
			defer mu.Unlock()
			volumes = append(volumes, db)
		},
		Stop: func() {
			mu.Lock()
			defer mu.Unlock()
			stopped = true
		},
	})

	// 0.05 min == 3s, just past the 2s fade configured above, so the fade
	// starts almost immediately and the whole cycle finishes quickly.
	if _, err := mgr.EmitCommand("sleep-timer", "set", []string{"0.05"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := stopped
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if !stopped {
		t.Fatal("playback was never stopped")
	}
	if len(volumes) < 3 {
		t.Fatalf("recorded %d volume changes, want a gradual fade", len(volumes))
	}
	// The fade descends, and the last write restores the original level.
	if volumes[1] >= volumes[0] {
		t.Errorf("volume did not descend during the fade: %v", volumes[:2])
	}
	if got := volumes[len(volumes)-1]; got != -5 {
		t.Errorf("final volume = %v, want the original -5 restored", got)
	}
}

func TestPomodoroCommands(t *testing.T) {
	mgr := loadStudyPlugins(t, nil, "pomodoro")

	if out, _ := mgr.EmitCommand("pomodoro", "status", nil); !strings.Contains(out, "off") {
		t.Errorf("status before start = %q, want off", out)
	}

	out, err := mgr.EmitCommand("pomodoro", "start", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !strings.Contains(out, "Focus") {
		t.Errorf("start = %q, want a focus phase", out)
	}

	if out, _ := mgr.EmitCommand("pomodoro", "stop", nil); !strings.Contains(out, "stopped") {
		t.Errorf("stop = %q", out)
	}
	if out, _ := mgr.EmitCommand("pomodoro", "status", nil); !strings.Contains(out, "off") {
		t.Errorf("status after stop = %q, want off", out)
	}
}

// The point of the feature: the break has to actually pause the music.
func TestPomodoroPausesPlaybackOnBreak(t *testing.T) {
	cfg := map[string]map[string]string{
		"pomodoro": {"work_minutes": "0.02", "break_minutes": "5"}, // 1.2s of "work"
	}
	mgr := loadStudyPlugins(t, cfg, "pomodoro")

	var mu sync.Mutex
	pauses := 0

	mgr.SetStateProvider(StateProvider{PlayerState: func() string { return "playing" }})
	mgr.SetControlProvider(ControlProvider{
		TogglePause: func() {
			mu.Lock()
			defer mu.Unlock()
			pauses++
		},
	})

	if _, err := mgr.EmitCommand("pomodoro", "start", nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := pauses
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if pauses == 0 {
		t.Fatal("work phase ended but playback was never paused for the break")
	}

	if out, _ := mgr.EmitCommand("pomodoro", "status", nil); !strings.Contains(out, "Break") {
		t.Errorf("status = %q, want a break phase after the work phase", out)
	}
}

// Completed focus rounds persist, so the count survives a restart.
func TestPomodoroPersistsCompletedRounds(t *testing.T) {
	cfg := map[string]map[string]string{
		"pomodoro": {"work_minutes": "0.02", "break_minutes": "5"},
	}
	mgr := loadStudyPlugins(t, cfg, "pomodoro")
	mgr.SetStateProvider(StateProvider{PlayerState: func() string { return "stopped" }})
	mgr.SetControlProvider(ControlProvider{TogglePause: func() {}})

	if _, err := mgr.EmitCommand("pomodoro", "start", nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if out, _ := mgr.EmitCommand("pomodoro", "status", nil); strings.Contains(out, "1 all time") {
			return // recorded
		}
		time.Sleep(150 * time.Millisecond)
	}
	out, _ := mgr.EmitCommand("pomodoro", "status", nil)
	t.Fatalf("completed round was never recorded; status = %q", out)
}

// The clock replaces the visualizer, so it must render at the panel's exact
// height and stay inside its width once escape codes are discounted.
func TestPomodoroRendersCountdown(t *testing.T) {
	mgr := loadStudyPlugins(t, nil, "pomodoro")

	if names := mgr.Visualizers(); len(names) != 1 || names[0] != "pomodoro" {
		t.Fatalf("Visualizers() = %v, want [pomodoro]", names)
	}

	var bands [10]float64
	const rows, cols = 16, 100

	idle := mgr.RenderVis("pomodoro", bands, rows, cols, 0)
	if idle == "" {
		t.Fatal("idle render is empty")
	}
	assertPanelFits(t, idle, rows, cols)
	// Cells are half-blocks shaded per pixel, so ▀ is the mark to look for.
	if !strings.Contains(idle, "▀") {
		t.Error("expected block digits in the idle render")
	}

	if _, err := mgr.EmitCommand("pomodoro", "start", nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	// RenderVis hands back the previous frame while the plugin is busy, and the
	// start command can still hold it for a moment, so wait for a fresh one.
	running := idle
	for frame, deadline := uint64(1), time.Now().Add(2*time.Second); running == idle && time.Now().Before(deadline); frame++ {
		running = mgr.RenderVis("pomodoro", bands, rows, cols, frame)
		if running == idle {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if running == idle {
		t.Error("render did not change once the session started")
	}
	assertPanelFits(t, running, rows, cols)
	if !strings.Contains(running, "─") && !strings.Contains(running, "━") {
		t.Error("expected a progress line under the clock")
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripClockMarker removes the "\0<time>\0" prefix the plugin emits to ask
// grbfy for an image clock. It is consumed before display, so it never counts
// towards the rendered width.
func stripClockMarker(out string) string {
	if !strings.HasPrefix(out, "\x00") {
		return out
	}
	if i := strings.Index(out[1:], "\x00"); i >= 0 {
		return out[i+2:]
	}
	return out
}

// assertPanelFits checks the render occupies exactly the panel it was given.
// Escape codes carry no width, so they are stripped before measuring.
func assertPanelFits(t *testing.T, out string, rows, cols int) {
	t.Helper()
	lines := strings.Split(stripClockMarker(out), "\n")
	if len(lines) != rows {
		t.Errorf("render has %d lines, want %d", len(lines), rows)
	}
	for i, line := range lines {
		if n := len([]rune(ansiRe.ReplaceAllString(line, ""))); n > cols {
			t.Errorf("line %d is %d cells wide, want <= %d", i, n, cols)
		}
	}
}

// The clock should grow to use the height it is given, not sit at one size.
func TestPomodoroClockScalesWithPanel(t *testing.T) {
	mgr := loadStudyPlugins(t, nil, "pomodoro")
	var bands [10]float64

	inkRows := func(out string) int {
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "▀") {
				n++
			}
		}
		return n
	}

	small := inkRows(mgr.RenderVis("pomodoro", bands, 9, 100, 0))
	large := inkRows(mgr.RenderVis("pomodoro", bands, 20, 100, 0))
	if large <= small {
		t.Errorf("clock used %d rows at height 20 vs %d at height 9; it should scale up", large, small)
	}
}

// Edges are smoothed by shading partly-covered pixels rather than snapping
// them on or off. Without this the curves show a visible staircase.
func TestPomodoroClockIsAntiAliased(t *testing.T) {
	mgr := loadStudyPlugins(t, map[string]map[string]string{
		"pomodoro": {"cell_aspect": "3.0"},
	}, "pomodoro")
	mgr.SetStateProvider(StateProvider{PlayerState: func() string { return "stopped" }})
	mgr.SetControlProvider(ControlProvider{TogglePause: func() {}})
	if _, err := mgr.EmitCommand("pomodoro", "start", nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	var bands [10]float64
	out := mgr.RenderVis("pomodoro", bands, 20, 164, 0)

	shade := regexp.MustCompile(`\x1b\[[34]8;2;(\d+);`)
	partial := 0
	for _, m := range shade.FindAllStringSubmatch(out, -1) {
		v, _ := strconv.Atoi(m[1])
		if v > 20 && v < 235 {
			partial++
		}
	}
	if partial < 50 {
		t.Errorf("only %d partly-shaded edge pixels; edges are not being smoothed", partial)
	}
}

// Rasterizing is the expensive part, so a repeat render of the same clock at
// the same size must come from cache rather than redoing the work.
func TestPomodoroClockCachesFrames(t *testing.T) {
	mgr := loadStudyPlugins(t, nil, "pomodoro")
	var bands [10]float64

	start := time.Now()
	mgr.RenderVis("pomodoro", bands, 20, 164, 0)
	cold := time.Since(start)

	start = time.Now()
	for range 20 {
		mgr.RenderVis("pomodoro", bands, 20, 164, 1)
	}
	warm := time.Since(start) / 20

	if warm > cold/4 {
		t.Errorf("cached render %v vs cold %v: frames are being rebuilt every tick", warm, cold)
	}
}

// Painting a black background behind a cell whose lower half is empty leaves a
// dark halo on any theme whose background is not pure black. A background is
// only legitimate where the lower half actually carries ink.
func TestPomodoroClockDoesNotPaintDarkBackgrounds(t *testing.T) {
	mgr := loadStudyPlugins(t, map[string]map[string]string{
		"pomodoro": {"cell_aspect": "3.0"},
	}, "pomodoro")
	mgr.SetStateProvider(StateProvider{PlayerState: func() string { return "stopped" }})
	mgr.SetControlProvider(ControlProvider{TogglePause: func() {}})
	if _, err := mgr.EmitCommand("pomodoro", "start", nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	var bands [10]float64
	out := mgr.RenderVis("pomodoro", bands, 20, 164, 0)

	bg := regexp.MustCompile(`\x1b\[48;2;(\d+);`)
	dark := 0
	for _, m := range bg.FindAllStringSubmatch(out, -1) {
		if v, _ := strconv.Atoi(m[1]); v < 40 {
			dark++
		}
	}
	if dark > 10 {
		t.Errorf("%d cells painted a near-black background; they read as shadows", dark)
	}
	if !strings.Contains(out, "\x1b[49m") {
		t.Error("expected the terminal's own background to be restored around strokes")
	}
}

// The clock must never be wider than its panel, at any size. The face has a
// minimum width, so narrow panels fall back to plain text instead.
func TestPomodoroClockNeverOverflows(t *testing.T) {
	mgr := loadStudyPlugins(t, map[string]map[string]string{"pomodoro": {"cell_aspect": "2"}}, "pomodoro")
	var bands [10]float64
	for rows := 1; rows <= 30; rows++ {
		for cols := 1; cols <= 120; cols++ {
			out := mgr.RenderVis("pomodoro", bands, rows, cols, 0)
			for i, line := range strings.Split(stripClockMarker(out), "\n") {
				if n := len([]rune(ansiRe.ReplaceAllString(line, ""))); n > cols {
					t.Fatalf("rows=%d cols=%d: line %d is %d cells wide", rows, cols, i, n)
				}
			}
		}
	}
}
