package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

// upNextModel builds a Model large enough to render an overlay body.
func upNextModel(t *testing.T, trackCount int, shuffle bool) *Model {
	t.Helper()
	withFrameWidth(t, 80)

	pl := playlist.New()
	tracks := make([]playlist.Track, trackCount)
	for i := range tracks {
		tracks[i] = playlist.Track{
			Path:  fmt.Sprintf("/tmp/track-%d.mp3", i),
			Title: fmt.Sprintf("Track %d", i+1),
		}
	}
	pl.Replace(tracks)
	if shuffle {
		pl.ToggleShuffle()
	}

	m := &Model{playlist: pl, width: 80, height: 40}
	m.recomputeLayout()
	return m
}

func upNextBodyText(m *Model) string {
	return stripAnsiRegExp.ReplaceAllString(m.renderUpNextBody(), "")
}

func TestUpNextBodyListsResolvedOrder(t *testing.T) {
	m := upNextModel(t, 6, false)

	body := upNextBodyText(m)
	// Sequential from the first track: the next ones are 2, 3, 4...
	for _, want := range []string{"1. Track 2", "2. Track 3", "3. Track 4"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	// The currently playing track is not "up next".
	if strings.Contains(body, "Track 1") {
		t.Errorf("body should not list the current track\n%s", body)
	}
}

func TestUpNextBodyMarksQueuedEntries(t *testing.T) {
	m := upNextModel(t, 6, false)
	m.playlist.Queue(4) // Track 5

	body := upNextBodyText(m)
	if !strings.Contains(body, "1. Track 5") {
		t.Errorf("queued track should be listed first\n%s", body)
	}
	if !strings.Contains(body, "[queued]") {
		t.Errorf("queued entry should carry the [queued] badge\n%s", body)
	}
}

func TestUpNextBodyShowsReshuffleMarker(t *testing.T) {
	m := upNextModel(t, 3, true)
	m.playlist.SetRepeat(playlist.RepeatAll)
	m.playlist.Next()
	m.playlist.Next()

	body := upNextBodyText(m)
	if !strings.Contains(body, "reshuffles from here") {
		t.Errorf("body should say the next order is not drawn yet\n%s", body)
	}
}

func TestUpNextBodyEmptyPlaylist(t *testing.T) {
	withFrameWidth(t, 80)
	m := &Model{playlist: playlist.New(), width: 80, height: 40}
	m.recomputeLayout()

	if body := upNextBodyText(m); !strings.Contains(body, "Nothing up next") {
		t.Errorf("empty playlist should say so\n%s", body)
	}
}

// A nil playlist must not panic: View can run before wiring completes.
func TestUpNextHandlesNilPlaylist(t *testing.T) {
	withFrameWidth(t, 80)
	m := &Model{width: 80, height: 40}
	m.recomputeLayout()

	if got := m.upNextHeaderLine(); got == "" {
		t.Error("header line should render with a nil playlist")
	}
	_ = m.renderUpNextBody()
	m.normalizeUpNextOverlay()
}

func TestUpNextKeysMoveCursorAndClose(t *testing.T) {
	m := upNextModel(t, 60, false)
	m.openUpNext()

	if !m.upNext.visible {
		t.Fatal("openUpNext did not open the overlay")
	}
	if m.activeScreen() != screenUpNext {
		t.Fatalf("activeScreen() = %v, want screenUpNext", m.activeScreen())
	}

	key := func(s string) tea.KeyPressMsg {
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}

	m.handleUpNextKey(key("j"))
	if m.upNext.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", m.upNext.cursor)
	}
	m.handleUpNextKey(key("k"))
	if m.upNext.cursor != 0 {
		t.Errorf("cursor after k = %d, want 0", m.upNext.cursor)
	}
	// Moving up from the top wraps to the end, matching the other lists.
	m.handleUpNextKey(key("k"))
	if m.upNext.cursor == 0 {
		t.Error("cursor at the top should wrap to the end")
	}

	m.handleUpNextKey(key("g"))
	if m.upNext.cursor != 0 {
		t.Errorf("g should return to the top, got %d", m.upNext.cursor)
	}
	m.handleUpNextKey(key("G"))
	if m.upNext.cursor != m.upNextCount()-1 {
		t.Errorf("G should go to the last entry, got %d of %d", m.upNext.cursor, m.upNextCount())
	}

	m.handleUpNextKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.upNext.visible {
		t.Error("Esc should close the overlay")
	}
}

// Reordering from the panel must change what actually plays next.
func TestUpNextReorderChangesPlayback(t *testing.T) {
	m := upNextModel(t, 8, false)
	m.openUpNext()

	before, _ := m.playlist.UpcomingWindow(2)
	m.handleUpNextKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})

	after, _ := m.playlist.UpcomingWindow(2)
	if after[0].Track.Title == before[0].Track.Title {
		t.Errorf("order unchanged after Shift+Down: still starts with %q", after[0].Track.Title)
	}
	if m.upNext.cursor != 1 {
		t.Errorf("cursor should follow the moved entry, got %d", m.upNext.cursor)
	}
}

// As playback consumes the list it gets shorter; a stale scroll offset must be
// pulled back into range rather than showing a blank panel.
func TestUpNextNormalizeClampsStaleScroll(t *testing.T) {
	m := upNextModel(t, 40, false)
	m.openUpNext()
	m.upNext.cursor, m.upNext.scroll = 39, 39

	m.playlist.Replace([]playlist.Track{{Path: "/tmp/a.mp3", Title: "A"}, {Path: "/tmp/b.mp3", Title: "B"}})
	m.normalizeUpNextOverlay()

	if m.upNext.cursor != 0 || m.upNext.scroll != 0 {
		t.Errorf("cursor/scroll = %d/%d, want clamped to 0 for a 1-entry list", m.upNext.cursor, m.upNext.scroll)
	}
}

func TestUpNextHeaderNamesPlaybackMode(t *testing.T) {
	m := upNextModel(t, 5, true)
	m.playlist.SetRepeat(playlist.RepeatAll)

	header := stripAnsiRegExp.ReplaceAllString(m.upNextHeaderLine(), "")
	for _, want := range []string{"Up Next", "shuffled", "repeat all"} {
		if !strings.Contains(header, want) {
			t.Errorf("header %q missing %q", header, want)
		}
	}

	seq := upNextModel(t, 5, false)
	if header := stripAnsiRegExp.ReplaceAllString(seq.upNextHeaderLine(), ""); !strings.Contains(header, "in order") {
		t.Errorf("header %q should say the order is sequential", header)
	}
}

func TestUpNextScreenLabel(t *testing.T) {
	if got := screenUpNext.label(); got != "Up Next" {
		t.Errorf("screenUpNext.label() = %q, want %q", got, "Up Next")
	}
}

// Removing an entry that comes from the order removes the track itself, which
// is what "remove" means for a song that is simply next in the playlist.
func TestUpNextRemovesOrderEntryFromPlaylist(t *testing.T) {
	m := upNextModel(t, 8, false)
	m.openUpNext()

	entry, ok := m.upNextEntryAt(0)
	if !ok {
		t.Fatal("no upcoming entry")
	}
	title := entry.Track.Title
	before := m.playlist.Len()

	m.handleUpNextKey(tea.KeyPressMsg{Code: 'd', Text: "d"})

	if got := m.playlist.Len(); got != before-1 {
		t.Fatalf("playlist length = %d, want %d", got, before-1)
	}
	after, _ := m.playlist.UpcomingWindow(1)
	if len(after) > 0 && after[0].Track.Title == title {
		t.Errorf("%q is still up next after being removed", title)
	}
	if !m.playlistUndo.active {
		t.Error("removal left no undo snapshot")
	}
}

// Removing a queued entry must leave the track in the playlist: it was only
// queued, not deleted.
func TestUpNextQueueRemovalKeepsTrack(t *testing.T) {
	m := upNextModel(t, 8, false)
	m.playlist.Queue(5)
	m.openUpNext()

	before := m.playlist.Len()
	m.handleUpNextKey(tea.KeyPressMsg{Code: 'd', Text: "d"})

	if m.playlist.QueueLen() != 0 {
		t.Errorf("queue length = %d, want 0", m.playlist.QueueLen())
	}
	if got := m.playlist.Len(); got != before {
		t.Errorf("playlist length = %d, want %d unchanged", got, before)
	}
	if !m.playlistUndo.active {
		t.Error("queue removal left no undo snapshot, but the message promises undo")
	}
}
