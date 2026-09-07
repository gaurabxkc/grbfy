package model

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

// upNextFetchCap bounds how far ahead the panel resolves. It is far more than
// fits on screen, so scrolling stays responsive, without walking a huge
// playlist on every frame.
const upNextFetchCap = 200

// upNextOverlay holds state for the Up Next panel.
type upNextOverlay struct {
	visible bool
	cursor  int
	scroll  int
}

func (m *Model) openUpNext() {
	m.upNext.visible = true
	m.upNext.cursor = 0
	m.upNext.scroll = 0
}

func (m *Model) upNextHelpLine() string {
	return m.commandHelp(commandModeUpNext)
}

func (m *Model) upNextVisible() int {
	return m.effectivePlaylistVisible()
}

// upNextHeaderLine names the playback mode, because the order shown only makes
// sense against it: "shuffle" explains why the list is not library order, and
// repeat explains why it wraps.
func (m *Model) upNextHeaderLine() string {
	label := "Up Next"
	if m.playlist == nil {
		return sepHeader(label)
	}
	mode := "in order"
	if m.playlist.Shuffled() {
		mode = "shuffled"
	}
	switch m.playlist.Repeat() {
	case playlist.RepeatAll:
		mode += " · repeat all"
	case playlist.RepeatOne:
		mode += " · repeat one"
	}
	return sepHeaderN(label+" · "+mode, m.upNext.cursor+1, m.upNextCount())
}

func (m Model) upNextCount() int {
	if m.playlist == nil {
		return 0
	}
	return m.playlist.UpcomingLen(upNextFetchCap)
}

// upNextEntryAt returns the entry the cursor is on, if the list still has one.
func (m Model) upNextEntryAt(n int) (playlist.UpcomingEntry, bool) {
	if m.playlist == nil || n < 0 {
		return playlist.UpcomingEntry{}, false
	}
	entries, _ := m.playlist.UpcomingWindow(upNextFetchCap)
	if n >= len(entries) {
		return playlist.UpcomingEntry{}, false
	}
	return entries[n], true
}

// upNextRows builds the display rows and reports whether the list ends at a
// point where the next order has not been drawn yet.
func (m Model) upNextRows() (rows []string, reshuffles bool) {
	if m.playlist == nil {
		return nil, false
	}
	entries, reshuffles := m.playlist.UpcomingWindow(upNextFetchCap)

	rows = make([]string, 0, len(entries)+1)
	for i, e := range entries {
		badge := ""
		if e.Queued {
			badge = " [queued]"
		}
		name := truncate(trackViewName(e.Track), max(1, ui.PanelWidth-10-len([]rune(badge))))
		row := fmt.Sprintf("%d. %s", i+1, name)
		if badge != "" {
			row += activeToggle.Render(badge)
		}
		rows = append(rows, row)
	}
	if reshuffles {
		rows = append(rows, dimStyle.Render("— reshuffles from here —"))
	}
	return rows, reshuffles
}

func (m Model) renderUpNextBody() string {
	budget := m.effectivePlaylistVisible()
	if budget <= 0 {
		return ""
	}
	rows, _ := m.upNextRows()
	if len(rows) == 0 {
		return bodyMessage("Nothing up next.", budget)
	}
	start := min(max(0, m.upNext.scroll), max(0, len(rows)-1))
	return windowList(rows, m.upNext.cursor, start, budget)
}

// normalizeUpNextOverlay keeps the cursor and scroll valid as playback advances
// and the list shrinks underneath them.
func (m *Model) normalizeUpNextOverlay() {
	count := m.upNextCount()
	if count <= 0 {
		m.upNext.cursor, m.upNext.scroll = 0, 0
		return
	}
	m.upNext.cursor = min(max(0, m.upNext.cursor), count-1)

	visible := m.upNextVisible()
	if visible <= 0 {
		m.upNext.scroll = min(max(0, m.upNext.scroll), count-1)
		return
	}
	clampScroll(&m.upNext.cursor, &m.upNext.scroll, count, visible)
}

// handleUpNextKey processes key presses while the Up Next panel is open.
func (m *Model) handleUpNextKey(msg tea.KeyPressMsg) tea.Cmd {
	count := m.upNextCount()
	visible := m.upNextVisible()

	switch msg.String() {
	case "esc", "b", "backspace", "U", "q":
		m.upNext.visible = false

	case "up", "k":
		if m.upNext.cursor > 0 {
			m.upNext.cursor--
		} else if count > 0 {
			m.upNext.cursor = count - 1
		}
	case "down", "j":
		if m.upNext.cursor < count-1 {
			m.upNext.cursor++
		} else {
			m.upNext.cursor = 0
		}
	case "pgup", "ctrl+u":
		m.upNext.cursor = max(0, m.upNext.cursor-visible)
	case "pgdown", "ctrl+d":
		m.upNext.cursor = min(max(0, count-1), m.upNext.cursor+visible)
	case "home", "g":
		m.upNext.cursor = 0
	case "end", "G":
		m.upNext.cursor = max(0, count-1)

	case "enter":
		if count == 0 {
			break
		}
		track, ok := m.playlist.JumpToUpcoming(m.upNext.cursor, upNextFetchCap)
		if !ok {
			break
		}
		m.upNext.visible = false
		m.plCursor = m.playlist.Index()
		m.adjustScroll()
		m.resetTitleScroll()
		m.status.Showf(statusTTLMedium, "Playing: %s", track.DisplayName())
		cmd := m.playTrack(track)
		m.notifyPlayback()
		return cmd

	// Shift+J/K mirror Shift+Down/Up so reordering follows the same hands as
	// j/k navigation.
	case "shift+up", "K":
		if idx, ok := m.playlist.MoveUpcoming(m.upNext.cursor, -1, upNextFetchCap); ok {
			m.upNext.cursor = idx
		} else {
			m.status.Show("Can't move past the queue boundary", statusTTLShort)
		}
	case "shift+down", "J":
		if idx, ok := m.playlist.MoveUpcoming(m.upNext.cursor, 1, upNextFetchCap); ok {
			m.upNext.cursor = idx
		} else {
			m.status.Show("Can't move past the queue boundary", statusTTLShort)
		}

	case "d":
		if count == 0 {
			break
		}
		// A queued entry is dropped from the queue. An entry that comes from
		// the order *is* the playlist, so removing it means removing the
		// track — done through the same path as `x`, so the saved playlist,
		// the undo snapshot and playback state all stay consistent.
		snapshot := m.playlist.Snapshot()
		if m.playlist.RemoveUpcoming(m.upNext.cursor, upNextFetchCap) {
			m.playlistUndo = playlistUndo{active: true, snapshot: snapshot}
			m.status.Show("Removed from queue (Ctrl+Z to undo)", statusTTLDefault)
			break
		}
		if entry, ok := m.upNextEntryAt(m.upNext.cursor); ok {
			m.removeTrackFromPlaylist(entry.TrackIndex)
		}

	default:
		// Skipping a track from here reshapes the very list being shown, so
		// the cursor still has to be re-clamped afterwards.
		cmd := m.transportKey(msg)
		m.normalizeUpNextOverlay()
		return cmd
	}

	m.normalizeUpNextOverlay()
	return nil
}
