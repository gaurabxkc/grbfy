package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The Cover visualizer: the playing track's album art, drawn as a real image
// where the terminal supports it (see cover.go), and the track's name where it
// does not — the same fallback the Lyrics mode uses.
type coverDriver struct {
	ctx VisCoverContext
}

func newCoverDriver() visModeDriver { return &coverDriver{} }

// AnalysisSpec is empty: this mode draws a picture, not audio.
func (*coverDriver) AnalysisSpec(*Visualizer) VisAnalysisSpec { return VisAnalysisSpec{} }

func (d *coverDriver) Tick(_ *Visualizer, ctx VisTickContext) {
	d.ctx = ctx.Cover
	SetCoverArt(ctx.Cover.ArtURL)
}

// A cover changes only with the track, so there is nothing to animate.
func (*coverDriver) TickInterval(*Visualizer, VisTickContext) time.Duration { return TickSlow }

func (*coverDriver) OnEnter(*Visualizer) {}
func (*coverDriver) OnLeave(*Visualizer) {}

func (d *coverDriver) Render(v *Visualizer) string {
	// Beside the track details when both fit: a square picture alone in a
	// short, full-width band reads as lost in the middle of the panel.
	if out, ok := d.renderWithDetails(v); ok {
		return out
	}
	if out, ok := RenderCover(v.Rows, PanelWidth); ok {
		return out
	}
	return centerTwoLines(v.Rows, d.ctx.TrackTitle, d.ctx.TrackArtist)
}

// coverDetailsMinWidth is the narrowest column worth giving the text. Below
// it the details are all ellipsis, so the cover takes the panel alone.
const coverDetailsMinWidth = 18

// coverGutter separates the art from the text.
const coverGutter = "  "

// renderWithDetails draws the cover on the left and the track's details
// beside it, the pair centred in the panel.
func (d *coverDriver) renderWithDetails(v *Visualizer) (string, bool) {
	art, artWidth, ok := RenderCoverBlock(v.Rows)
	if !ok {
		return "", false
	}
	textWidth := PanelWidth - artWidth - len(coverGutter)
	if textWidth < coverDetailsMinWidth {
		return "", false
	}

	details := d.details(textWidth)
	pad := max(0, (PanelWidth-artWidth-len(coverGutter)-longestWidth(details))/2)
	lead := strings.Repeat(" ", pad)

	// The text block sits centred against the art rather than at its top.
	top := max(0, (len(art)-len(details))/2)
	out := make([]string, len(art))
	for i, row := range art {
		line := lead + row
		if i >= top && i-top < len(details) {
			line += coverGutter + details[i-top]
		}
		out[i] = line
	}
	return strings.Join(out, "\n"), true
}

// details is the track's text beside the art: what it is, then where it comes
// from. Empty fields are dropped rather than leaving blank rows.
func (d *coverDriver) details(width int) []string {
	lines := make([]string, 0, 3)
	add := func(style lipgloss.Style, text string) {
		if text == "" {
			return
		}
		lines = append(lines, style.Render(ansi.Truncate(text, width, "…")))
	}
	add(lyricsCurrentStyle, d.ctx.TrackTitle)
	add(lyricsNextStyle, d.ctx.TrackArtist)
	add(lyricsNextStyle, d.ctx.AlbumLine)
	return lines
}

// longestWidth is the visible width of the widest line.
func longestWidth(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest
}

// VisCoverContext is what the Cover visualizer draws from, resolved by the
// model once per tick so the driver never reaches into playlist state.
type VisCoverContext struct {
	// ArtURL is the playing track's artwork: an http(s) URL from a provider,
	// or a file:// URL for art cached from a local file's tags.
	ArtURL      string
	TrackTitle  string
	TrackArtist string
	// AlbumLine is the album with its year when there is one, already
	// formatted by the model: the driver does no field assembly.
	AlbumLine string
}
