package model

import "strings"

// toggleFullVisQueue shows or hides the up-next list inside the fullscreen
// visualizer, so what plays next can be checked without leaving it.
func (m *Model) toggleFullVisQueue() {
	m.fullVisQueue = !m.fullVisQueue
	m.vis.RequestRefresh()
	m.refreshChrome()
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
	for _, row := range entries {
		if len(lines) >= rows {
			break
		}
		lines = append(lines, "  "+row)
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
		return helpKey("u", "Vis ")
	}
	return helpKey("u", "Queue ")
}
