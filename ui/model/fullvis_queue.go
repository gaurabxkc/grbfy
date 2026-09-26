package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// toggleFullVisQueue shows or hides the up-next list inside the fullscreen
// visualizer, so what plays next can be checked and changed without leaving
// it. The list is the U overlay's own: the same cursor, the same actions.
func (m *Model) toggleFullVisQueue() {
	m.fullVisQueue = !m.fullVisQueue
	if m.fullVisQueue {
		m.upNext.cursor, m.upNext.scroll = 0, 0
	}
	m.vis.RequestRefresh()
	m.refreshChrome()
}

// handleFullVisQueueKey runs a key against the fullscreen up-next list. It
// reports false for the keys that still belong to the fullscreen view, so
// switching modes, hiding the title, leaving and the keymap keep working with
// the list open.
func (m *Model) handleFullVisQueueKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc", "U", "b", "backspace", "q":
		// Back out of the list first; a second Esc leaves the screen.
		m.toggleFullVisQueue()
		return nil, true
	case "V", "v", "t", "ctrl+k", "?", "left", "right":
		return nil, false
	}

	// handleUpNextKey closes the overlay when Enter plays a track (its only
	// remaining way to close, since the close keys are taken above); here that
	// means returning to the visualizer. The overlay flag is set just for the
	// call and cleared after, so the list never reappears outside full screen.
	m.upNext.visible = true
	cmd := m.handleUpNextKey(msg)
	if !m.upNext.visible {
		m.fullVisQueue = false
	}
	m.upNext.visible = false
	m.vis.RequestRefresh()
	return cmd, true
}

// renderFullVisQueue draws the upcoming tracks in the space the spectrum
// normally occupies, keeping the fullscreen layout the same height whether
// the list is showing or not — the rows are the visualizer's own budget, so
// nothing above or below shifts when it is toggled.
//
// It reuses upNextRows, the same renderer the U overlay uses, so queued
// entries carry the same [queued] badge and the shuffle-wrap note appears
// in both places rather than drifting apart.
func (m Model) renderFullVisQueue() string {
	rows := m.vis.Rows
	if rows <= 0 {
		return ""
	}

	lines := make([]string, 0, rows)
	lines = append(lines, dimStyle.Render(labeledSeparator("", "Up Next")))

	entries, _ := m.upNextRows()
	if len(entries) == 0 {
		lines = append(lines, dimStyle.Render("  Nothing up next."))
	}
	// The window follows the cursor, sized to this screen rather than to the
	// playlist pane the overlay's own scroll is kept against.
	budget := rows - len(lines)
	if len(entries) > 0 && budget > 0 {
		cursor := min(max(0, m.upNext.cursor), len(entries)-1)
		start := max(0, cursor-budget+1)
		for _, line := range strings.Split(windowList(entries, cursor, start, budget), "\n") {
			lines = append(lines, line)
		}
	}

	// Pad to the visualizer's height so the seek bar and help line stay put.
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return strings.Join(fitLines(lines[:rows], rows), "\n")
}

// fullVisSpectrumOrQueue picks what fills the fullscreen visualizer's main
// area: the spectrum, or the up-next list when it has been toggled on.
func (m Model) fullVisSpectrumOrQueue() string {
	if m.fullVisQueue {
		return m.renderFullVisQueue()
	}
	return m.renderSpectrum()
}

// fullVisQueueHelp labels the toggle in the fullscreen help line.
func (m Model) fullVisQueueHelp() string {
	if m.fullVisQueue {
		return helpKey("U", "Vis ")
	}
	return helpKey("U", "Queue ")
}

// fullVisHelpLine is the fullscreen help line: the list's own keys while it
// is open, the visualizer's otherwise.
func (m Model) fullVisHelpLine() string {
	if m.fullVisQueue {
		return helpKey("↑↓", "Move ") + helpKey("Enter", "Play ") + helpKey("J K", "Reorder ") +
			helpKey("d", "Remove ") + helpKey("Esc", "Back ") + helpKey("V", "Exit ") + helpKey("?", "Keys")
	}
	return helpKey("V", "Exit ") + helpKey("v", "Mode:"+m.vis.ModeName()+" ") + m.fullVisQueueHelp() +
		helpKey("Spc", "▶❚❚ ") + helpKey("<>", "Trk ") + helpKey("+-", "Vol ") + helpKey("t", "Title ") + helpKey("?", "Keys")
}
