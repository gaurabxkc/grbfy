package ui

import (
	"fmt"
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

// Nothing animates, but the progress bar has to move: a second is the
// coarsest tick that still keeps the elapsed time honest.
func (*coverDriver) TickInterval(*Visualizer, VisTickContext) time.Duration {
	return time.Second
}

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

// The progress bar's glyphs match the player's own seek bar.
const (
	coverBarFill     = "━"
	coverBarHead     = "╸"
	coverBarEmpty    = "─"
	coverBarMinWidth = 8
)

var (
	coverBarFillStyle  = lipgloss.NewStyle()
	coverBarEmptyStyle = lipgloss.NewStyle().Faint(true)
	coverNextStyle     = lipgloss.NewStyle().Faint(true)
)

// renderWithDetails is the now-playing screen: the cover on the left, the
// track's details beside it, and what plays next underneath.
func (d *coverDriver) renderWithDetails(v *Visualizer) (string, bool) {
	// The next-track line gets its own row below the art when there is one to
	// spare, so it reads as a footnote rather than part of the block.
	artRows := v.Rows
	next := d.nextLine()
	if next != "" && artRows >= coverNextMinRows {
		artRows--
	} else {
		next = ""
	}

	art, artWidth, ok := RenderCoverBlock(artRows)
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
	out := make([]string, 0, v.Rows)
	for i, row := range art {
		line := lead + row
		if i >= top && i-top < len(details) {
			line += coverGutter + details[i-top]
		}
		out = append(out, line)
	}
	if next != "" {
		out = append(out, centerInPanel(coverNextStyle.Render(ansi.Truncate(next, PanelWidth, "…"))))
	}
	for len(out) < v.Rows {
		out = append(out, "")
	}
	return strings.Join(out[:v.Rows], "\n"), true
}

// coverNextMinRows is the height below which the next-track line costs more
// than it is worth: the row comes out of the picture.
const coverNextMinRows = 7

// details is what sits beside the art: the track, then where it comes from,
// then how far through it is. Empty fields are dropped rather than leaving
// blank rows, and the progress bar only appears when it has room and a
// duration to work from.
func (d *coverDriver) details(width int) []string {
	lines := make([]string, 0, 5)
	add := func(style lipgloss.Style, text string) {
		if text == "" {
			return
		}
		lines = append(lines, style.Render(ansi.Truncate(text, width, "…")))
	}
	add(lyricsCurrentStyle, d.ctx.TrackTitle)
	add(lyricsNextStyle, d.ctx.TrackArtist)
	add(lyricsNextStyle, d.ctx.AlbumLine)

	if bar := d.progressLine(width); bar != "" {
		lines = append(lines, "", bar)
	}
	return lines
}

// nextLine is the "next · Outlier — Bonobo" footnote, empty when nothing
// follows this track.
func (d *coverDriver) nextLine() string {
	if d.ctx.NextLine == "" {
		return ""
	}
	return "next · " + d.ctx.NextLine
}

// progressLine is "01:12 ━━━━━╸───── 03:45", tinted with the cover's own
// accent colour so the screen belongs to the record playing. It is dropped
// for live streams and anything with no known duration, where a bar would be
// a lie, and in columns too narrow to show one.
func (d *coverDriver) progressLine(width int) string {
	if d.ctx.DurationSecs <= 0 {
		return ""
	}
	elapsed := formatCoverTime(d.ctx.PositionSecs)
	total := formatCoverTime(d.ctx.DurationSecs)
	barWidth := width - lipgloss.Width(elapsed) - lipgloss.Width(total) - 2
	if barWidth < coverBarMinWidth {
		return ""
	}

	progress := max(0, min(1, float64(d.ctx.PositionSecs)/float64(d.ctx.DurationSecs)))
	filled := min(int(progress*float64(barWidth)), barWidth)

	fill := coverBarFillStyle
	if tint, ok := CoverAccent(); ok {
		fill = fill.Foreground(tint)
	}

	var b strings.Builder
	b.WriteString(lyricsNextStyle.Render(elapsed))
	b.WriteString(" ")
	if filled >= barWidth {
		b.WriteString(fill.Render(strings.Repeat(coverBarFill, barWidth)))
	} else {
		b.WriteString(fill.Render(strings.Repeat(coverBarFill, filled) + coverBarHead))
		b.WriteString(coverBarEmptyStyle.Render(strings.Repeat(coverBarEmpty, barWidth-filled-1)))
	}
	b.WriteString(" ")
	b.WriteString(lyricsNextStyle.Render(total))
	return b.String()
}

// formatCoverTime renders seconds as m:ss, or h:mm:ss past an hour.
func formatCoverTime(secs int) string {
	if secs < 0 {
		secs = 0
	}
	if h := secs / 3600; h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, (secs%3600)/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
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
	// PositionSecs and DurationSecs drive the progress bar. A zero duration
	// (a live stream) means no bar rather than an empty one.
	PositionSecs int
	DurationSecs int
	// NextLine is what plays next, already formatted by the model.
	NextLine string
	// AlbumLine is the album with its year when there is one, already
	// formatted by the model: the driver does no field assembly.
	AlbumLine string
}
