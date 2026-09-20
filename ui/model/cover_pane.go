package model

import (
	"strconv"
	"strings"

	"github.com/bjarneo/cliamp/ui"
)

// The album art sits at the top of the settings pane: it is the one column
// whose width is stable across layouts, and the cover reads as part of the
// now-playing chrome there rather than floating in the visualizer band.
//
// coverPaneMinSettingsRows keeps the settings themselves usable. Below this
// the pane is too short to give any of it away, so the cover is dropped
// rather than squeezing the controls off-screen.
const coverPaneMinSettingsRows = 4

// coverPaneRows is how many rows the cover takes in a pane of the given
// height, or 0 when it does not fit, is switched off, or the track has no
// artwork ready.
func (m Model) coverPaneRows(rows int) int {
	if !m.showCover || !m.layout.twoColumn {
		return 0
	}
	// The pane is a consumer of the artwork in its own right: the Cover
	// visualizer may never run, and the fetch has to start from whoever wants
	// the picture first. SetCoverArt only acts on a change.
	ui.SetCoverArt(m.visualizerCoverContext().ArtURL)

	want := ui.CoverRowsFor(m.layout.settingsWidth)
	if want == 0 {
		return 0
	}
	// One row goes to the separator below the picture.
	if rows-want-1 < coverPaneMinSettingsRows {
		want = rows - 1 - coverPaneMinSettingsRows
	}
	if want < 3 {
		return 0
	}
	return want
}

// renderSettingsPaneWithCover draws the artwork above the settings when there
// is room for both. The settings pane is rendered against the rows left over,
// so its own layout decisions (which controls to drop first) still apply.
func (m Model) renderSettingsPaneWithCover(rows int) string {
	coverRows := m.coverPaneRows(rows)
	if coverRows == 0 {
		return m.renderSettingsPane(rows)
	}

	art, ok := ui.RenderCover(coverRows, m.layout.settingsWidth)
	if !ok {
		return m.renderSettingsPane(rows)
	}

	lines := strings.Split(art, "\n")
	lines = append(lines, fillSeparator(sepHeader("Settings"), m.layout.settingsWidth))
	lines = append(lines, strings.Split(m.renderSettingsPane(rows-coverRows-1), "\n")...)
	return strings.Join(lines, "\n")
}

// toggleCover shows or hides the artwork and remembers the choice, the way
// the metadata pane does.
func (m *Model) toggleCover() {
	m.SetShowCover(!m.showCover)
	m.saveConfigKey("show_cover", strconv.FormatBool(m.showCover))

	art := m.visualizerCoverContext().ArtURL
	ui.SetCoverArt(art)
	switch {
	case !m.showCover:
		m.status.Show("Album art hidden", statusTTLShort)
	case !ui.ClockGraphicsAvailable():
		m.status.Show("Album art needs a terminal with image support", statusTTLDefault)
	case !m.layout.twoColumn:
		m.status.Show("Album art needs a wider terminal", statusTTLDefault)
	case art == "":
		m.status.Show("No album art for this track", statusTTLDefault)
	case ui.CoverRowsFor(m.layout.settingsWidth) == 0:
		// The fetch is in flight; the next frame draws it.
		m.status.Show("Album art: loading…", statusTTLShort)
	default:
		m.status.Show("Album art shown", statusTTLShort)
	}
}
