package model

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

// volumeSpyEngine records volume changes, which the shared fake discards.
type volumeSpyEngine struct {
	playbackFakeEngine
	vol float64
}

func (e *volumeSpyEngine) SetVolume(v float64) { e.vol = v }
func (e *volumeSpyEngine) Volume() float64     { return e.vol }

// transportModel builds a model with a playlist and a stub engine, big enough
// for the overlay handlers to compute their own geometry.
func transportModel(t *testing.T) (*Model, *volumeSpyEngine) {
	t.Helper()
	withFrameWidth(t, 80)

	pl := playlist.New()
	tracks := make([]playlist.Track, 8)
	for i := range tracks {
		tracks[i] = playlist.Track{
			Path:  fmt.Sprintf("/tmp/track-%d.mp3", i),
			Title: fmt.Sprintf("Track %d", i+1),
		}
	}
	pl.Replace(tracks)

	engine := &volumeSpyEngine{}
	m := &Model{playlist: pl, player: engine, width: 80, height: 40}
	m.recomputeLayout()
	return m, engine
}

func charKey(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

// The complaint this fixes: opening a list used to cost you the transport.
func TestOverlayHandlersKeepTransportLive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		handle func(*Model, tea.KeyPressMsg) tea.Cmd
	}{
		{"up next", (*Model).handleUpNextKey},
		{"queue", (*Model).handleQueueKey},
		{"keymap", (*Model).handleKeymapKey},
		{"theme picker", (*Model).handleThemeKey},
		{"visualizer picker", (*Model).handleVisPickerKey},
		{"device picker", (*Model).handleDeviceKey},
		{"playlist picker", (*Model).handlePlaylistPickerKey},
		{"playlist manager", (*Model).handlePlMgrListKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, engine := transportModel(t)

			tc.handle(m, charKey("."))
			if got := m.playlist.Index(); got != 1 {
				t.Errorf("`.` left the playlist at index %d, want 1: next track must still work here", got)
			}
			tc.handle(m, charKey(","))
			if got := m.playlist.Index(); got != 0 {
				t.Errorf("`,` left the playlist at index %d, want 0", got)
			}

			tc.handle(m, charKey("+"))
			if engine.vol != 1 {
				t.Errorf("volume after `+` = %v, want 1", engine.vol)
			}
			tc.handle(m, charKey("-"))
			if engine.vol != 0 {
				t.Errorf("volume after `-` = %v, want 0", engine.vol)
			}
		})
	}
}

// The fallback must never outrank an overlay's own meaning for a key. The file
// browser is the case that proves it: it binds both `.` (jump to the working
// directory) and Space (mark an entry), and must keep them.
func TestOverlayOwnBindingsBeatTransport(t *testing.T) {
	m, engine := transportModel(t)
	m.openFileBrowser()

	m.handleFileBrowserKey(charKey("."))
	m.handleFileBrowserKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if got := m.playlist.Index(); got != 0 {
		t.Errorf("playlist index = %d, want 0: the file browser owns `.` and Space", got)
	}

	// Keys it does not bind still reach the transport.
	m.handleFileBrowserKey(charKey("+"))
	if engine.vol != 1 {
		t.Errorf("volume after `+` in the file browser = %v, want 1", engine.vol)
	}
}

func TestUpNextReorderAcceptsShiftJK(t *testing.T) {
	m, _ := transportModel(t)
	m.openUpNext()
	m.playlist.Queue(4) // Track 5
	m.playlist.Queue(2) // Track 3

	first := func() string {
		entries, _ := m.playlist.UpcomingWindow(upNextFetchCap)
		return entries[0].Track.Title
	}
	if first() != "Track 5" {
		t.Fatalf("setup: first upcoming = %q", first())
	}

	m.handleUpNextKey(charKey("J")) // same as Shift+Down
	if got := first(); got != "Track 3" {
		t.Errorf("first upcoming after J = %q, want Track 3", got)
	}
	m.upNext.cursor = 1
	m.handleUpNextKey(charKey("K")) // same as Shift+Up
	if got := first(); got != "Track 5" {
		t.Errorf("first upcoming after K = %q, want Track 5", got)
	}
}

func TestQueueReorderAcceptsShiftJK(t *testing.T) {
	m, _ := transportModel(t)
	m.playlist.Queue(4) // Track 5
	m.playlist.Queue(2) // Track 3
	m.queue.visible = true

	m.handleQueueKey(charKey("J"))
	if m.queue.cursor != 1 {
		t.Errorf("cursor after J = %d, want 1", m.queue.cursor)
	}
	entries, _ := m.playlist.UpcomingWindow(upNextFetchCap)
	if entries[0].Track.Title != "Track 3" {
		t.Errorf("queue head after J = %q, want Track 3", entries[0].Track.Title)
	}

	m.handleQueueKey(charKey("K"))
	entries, _ = m.playlist.UpcomingWindow(upNextFetchCap)
	if entries[0].Track.Title != "Track 5" {
		t.Errorf("queue head after K = %q, want Track 5", entries[0].Track.Title)
	}
}

// Plugins must not be able to bind a key the overlays now answer to.
func TestTransportKeysAreReserved(t *testing.T) {
	reserved := ReservedKeys()
	for _, key := range []string{
		"space", ">", ".", "<", ",", "+", "=", "-", "shift+left", "shift+right", "J", "K",
	} {
		if !reserved[key] {
			t.Errorf("%q is answered by the core UI but not reserved: a plugin could bind over it", key)
		}
	}
}
