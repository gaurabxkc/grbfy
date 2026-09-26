package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/ui"
)

func TestFullVisQueueTogglesAndKeepsHeight(t *testing.T) {
	m := &Model{}
	if m.fullVisQueue {
		t.Fatal("the fullscreen queue should start hidden")
	}

	m.toggleFullVisQueue()
	if !m.fullVisQueue {
		t.Error("toggleFullVisQueue() did not turn the queue on")
	}
	m.toggleFullVisQueue()
	if m.fullVisQueue {
		t.Error("toggleFullVisQueue() did not turn the queue back off")
	}
}

func TestRenderFullVisQueueFillsExactlyTheVisualizerHeight(t *testing.T) {
	m := Model{}
	m.vis = ui.NewVisualizer(44100)
	m.vis.Rows = 6

	got := m.renderFullVisQueue()
	if n := len(strings.Split(got, "\n")); n != m.vis.Rows {
		t.Errorf("rendered %d rows, want %d — the seek bar below must not shift", n, m.vis.Rows)
	}
}

func TestFullVisQueueHelpLabelsTheOtherState(t *testing.T) {
	m := Model{}
	off := m.fullVisQueueHelp()
	m.fullVisQueue = true
	on := m.fullVisQueueHelp()
	if off == on {
		t.Error("the help label should say what U will do next, not the current state")
	}
}

// fullVisListModel is a fullscreen visualizer with the up-next list open over
// a ten-track playlist, the first track playing.
func fullVisListModel(t *testing.T) *Model {
	t.Helper()
	m := upNextModel(t, 10, false)
	m.player = &playbackFakeEngine{playing: true}
	m.vis = ui.NewVisualizer(44100)
	m.vis.Rows = 6
	m.playlist.SetIndex(0)
	m.fullVis = true
	m.handleFullVisualizerKey(tea.KeyPressMsg{Code: 'U', Text: "U"})
	if !m.fullVisQueue {
		t.Fatal("U did not open the up-next list in full screen")
	}
	return m
}

func fvKey(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// U is the up-next key everywhere else, so it is here too; the old u is not.
func TestFullVisQueueOpensOnShiftU(t *testing.T) {
	m := upNextModel(t, 3, false)
	m.vis = ui.NewVisualizer(44100)
	m.fullVis = true

	m.handleFullVisualizerKey(fvKey("u"))
	if m.fullVisQueue {
		t.Fatal("lowercase u still opens the list")
	}
	m.handleFullVisualizerKey(fvKey("U"))
	if !m.fullVisQueue {
		t.Fatal("U did not open the list")
	}
}

// Esc backs out one level at a time: the list first, then the screen.
func TestFullVisEscClosesTheListBeforeTheScreen(t *testing.T) {
	m := fullVisListModel(t)

	m.handleFullVisualizerKey(fvKey("esc"))
	if m.fullVisQueue {
		t.Fatal("Esc did not close the list")
	}
	if !m.fullVis {
		t.Fatal("Esc left full screen instead of just closing the list")
	}

	m.handleFullVisualizerKey(fvKey("esc"))
	if m.fullVis {
		t.Fatal("a second Esc did not leave full screen")
	}
}

// The list takes the same actions as the U overlay outside.
func TestFullVisQueueHasTheOverlaysActions(t *testing.T) {
	m := fullVisListModel(t)
	before := m.upNextCount()

	m.handleFullVisualizerKey(fvKey("down"))
	if m.upNext.cursor != 1 {
		t.Fatalf("down moved the cursor to %d, want 1", m.upNext.cursor)
	}

	second, _ := m.upNextEntryAt(1)
	m.handleFullVisualizerKey(fvKey("K"))
	if m.upNext.cursor != 0 {
		t.Fatalf("K left the cursor at %d, want the moved entry at 0", m.upNext.cursor)
	}
	if moved, _ := m.upNextEntryAt(0); moved.Track.Path != second.Track.Path {
		t.Fatalf("K did not move %q to the top", second.Track.Path)
	}

	m.handleFullVisualizerKey(fvKey("d"))
	if m.upNextCount() != before-1 {
		t.Fatalf("d left %d upcoming, want %d", m.upNextCount(), before-1)
	}

	if !m.fullVisQueue || !m.fullVis {
		t.Fatal("list actions closed the list or the screen")
	}
	if m.upNext.visible {
		t.Fatal("the U overlay flag leaked; the list would reappear outside full screen")
	}
}

// Enter plays the selected track and returns to the visualizer, as the
// overlay closes after Enter outside.
func TestFullVisQueueEnterPlaysAndReturnsToTheVisualizer(t *testing.T) {
	m := fullVisListModel(t)
	m.handleFullVisualizerKey(fvKey("down"))
	want, _ := m.upNextEntryAt(1)

	m.handleFullVisualizerKey(fvKey("enter"))
	if cur, _ := m.playlist.Current(); cur.Path != want.Track.Path {
		t.Fatalf("playing %q, want the selected %q", cur.Path, want.Track.Path)
	}
	if m.fullVisQueue {
		t.Fatal("the list stayed open after playing a track")
	}
	if !m.fullVis {
		t.Fatal("Enter left full screen")
	}
}

// The screen's own keys keep working with the list open.
func TestFullVisQueueKeepsTheScreensOwnKeys(t *testing.T) {
	m := fullVisListModel(t)
	mode := m.vis.ModeName()
	m.handleFullVisualizerKey(fvKey("v"))
	if m.vis.ModeName() == mode {
		t.Fatal("v did not change the visualizer mode with the list open")
	}
	m.handleFullVisualizerKey(fvKey("V"))
	if m.fullVis {
		t.Fatal("V did not leave full screen with the list open")
	}
	if m.fullVisQueue {
		t.Fatal("leaving full screen kept the list open for next time")
	}
}

// The window follows the cursor down a list longer than the screen.
func TestFullVisQueueWindowFollowsTheCursor(t *testing.T) {
	m := fullVisListModel(t)
	for range 8 {
		m.handleFullVisualizerKey(fvKey("down"))
	}
	out := stripAnsiRegExp.ReplaceAllString(m.renderFullVisQueue(), "")
	if n := len(strings.Split(out, "\n")); n != m.vis.Rows {
		t.Fatalf("rendered %d rows, want %d", n, m.vis.Rows)
	}
	sel, _ := m.upNextEntryAt(m.upNext.cursor)
	if !strings.Contains(out, sel.Track.Title) {
		t.Fatalf("the selected %q scrolled out of view:\n%s", sel.Track.Title, out)
	}
}
