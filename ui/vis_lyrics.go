package ui

import (
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The Lyrics visualizer is Go rather than a Lua plugin, against this fork's
// usual preference, for one reason: the Lua visualizer API exposes playback
// position and track metadata but no lyrics at all. A Lua version would have
// to re-implement LRCLIB/NetEase fetching, LRC parsing and caching from
// scratch and would keep its own lyrics state alongside the app's. Reading
// the already-loaded lines is both smaller and correct.

// lyricsNoteGlyph stands in for a line with no text — an instrumental break
// in a synced file. The lyrics overlay uses the same substitution.
const lyricsNoteGlyph = "♪"

type lyricsDriver struct {
	ctx VisLyricsContext
}

func newLyricsDriver() visModeDriver { return &lyricsDriver{} }

// AnalysisSpec is empty: this mode draws text on a clock, not audio.
func (*lyricsDriver) AnalysisSpec(*Visualizer) VisAnalysisSpec { return VisAnalysisSpec{} }

func (d *lyricsDriver) Tick(_ *Visualizer, ctx VisTickContext) {
	// Replace wholesale rather than merging, so a track change can never
	// leave the previous song's line on screen.
	d.ctx = ctx.Lyrics
}

// TickInterval stays slow: lyric lines turn over every few seconds, and
// nothing here animates between them.
func (*lyricsDriver) TickInterval(*Visualizer, VisTickContext) time.Duration { return TickSlow }

func (*lyricsDriver) OnEnter(*Visualizer) {}
func (*lyricsDriver) OnLeave(*Visualizer) {}

func (d *lyricsDriver) Render(v *Visualizer) string {
	primary, secondary := d.lines()
	if big, ok := renderBigLyricLine(v, primary); ok {
		return big
	}
	return centerTwoLines(v.Rows, primary, secondary)
}

// lines picks what to draw: the synced current/next line when there is one,
// otherwise the track's title and artist. Falling back to track info rather
// than blanking keeps the mode from looking broken on radio streams and on
// tracks with no lyrics match.
func (d *lyricsDriver) lines() (primary, secondary string) {
	if d.ctx.Syncable && d.ctx.HasLines && d.ctx.CurrentLine != "" {
		next := d.ctx.NextLine
		if next == "" {
			next = lyricsNoteGlyph
		}
		return d.ctx.CurrentLine, next
	}
	if d.ctx.TrackTitle == "" && d.ctx.TrackArtist == "" {
		return lyricsNoteGlyph, ""
	}
	return d.ctx.TrackTitle, d.ctx.TrackArtist
}

// --- Small-text fallback: plain centered lines, used whenever the text
// isn't drawable in the big pixel font below. ---

// centerTwoLines places primary on the middle row and secondary just below
// it, both horizontally centred in the panel. There is no shared text
// centring helper in this package — the other text-ish modes draw bitmaps
// into a Braille grid instead — so this does the padding itself.
func centerTwoLines(rows int, primary, secondary string) string {
	if rows <= 0 {
		return ""
	}
	out := make([]string, rows)
	mid := rows / 2
	for row := range rows {
		switch row {
		case mid:
			out[row] = centerInPanel(lyricsCurrentStyle.Render(truncateToPanel(primary)))
		case mid + 1:
			if secondary != "" {
				out[row] = centerInPanel(lyricsNextStyle.Render(truncateToPanel(secondary)))
			}
		}
	}
	return strings.Join(out, "\n")
}

// truncateToPanel clips a lyric line to the panel width without splitting an
// escape sequence or a wide rune. Long lines are common; wrapping them would
// shift the vertical centring, so they are cut instead.
func truncateToPanel(s string) string {
	if PanelWidth <= 0 {
		return ""
	}
	return ansi.Truncate(s, PanelWidth, "…")
}

// centerInPanel pads a rendered (possibly styled) string so its visible width
// sits centred in PanelWidth. Padding leads only — trailing spaces would show
// up as selection artefacts.
func centerInPanel(s string) string {
	pad := (PanelWidth - ansi.StringWidth(s)) / 2
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}

var (
	lyricsCurrentStyle = lipgloss.NewStyle().Bold(true)
	lyricsNextStyle    = lipgloss.NewStyle().Faint(true)
)

// --- Big pixel-art text: the "big lyrics" mode proper. ---

const (
	bigFontW        = 5 // pixel columns per glyph
	bigFontH        = 7 // pixel rows per glyph
	bigFontGap      = 1 // pixel columns between glyphs
	bigFontLineGap  = 2 // pixel rows between wrapped text rows
	bigFontMaxScale = 6 // cap on how large a single short word may get
	bigFontMaxLines = 4 // give up wrapping beyond this; caller falls back
)

// bigFontGlyphs holds 5×7 pixel bitmaps for the big-lyrics font: uppercase
// A-Z, 0-9, and the punctuation lyrics actually use. Each row is 5 bits
// wide; bit 4 (0x10) is the leftmost pixel, matching logoGlyphs in
// vis_logo.go. Lowercase input is upper-cased before lookup — one case is
// enough for a display font and halves the glyphs to hand-author.
//
// There is no equivalent for Devanagari or other complex scripts: their
// glyph shapes (conjuncts, matras) aren't reasonably hand-authorable as a
// 5×7 bitmap. Text containing anything outside this set is rejected by
// bigFontSupports rather than drawn with gaps, and the caller falls back to
// normal small text instead.
var bigFontGlyphs = map[rune][bigFontH]uint8{
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0F, 0x10, 0x10, 0x10, 0x10, 0x10, 0x0F},
	'D': {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0F, 0x10, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x1F},
	'J': {0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x11, 0x11, 0x11, 0x11},
	'N': {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},

	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},

	'\'': {0x08, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00},
	',':  {0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x08},
	'.':  {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04},
	'!':  {0x04, 0x04, 0x04, 0x04, 0x04, 0x00, 0x04},
	'?':  {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
	'-':  {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
}

// bigFontSupports reports whether every non-space rune in s (upper-cased)
// has a glyph. A single unsupported rune rejects the whole line — a partly
// pixel-art, partly-blank line would look broken, not intentional.
func bigFontSupports(s string) bool {
	for _, r := range s {
		if r == ' ' {
			continue
		}
		if _, ok := bigFontGlyphs[unicode.ToUpper(r)]; !ok {
			return false
		}
	}
	return true
}

// renderBigLyricLine draws text as large pixel-art letters, choosing the
// largest scale that still lets it wrap into bigFontMaxLines rows or fewer
// within the panel. It reports false for empty text or text using any rune
// outside bigFontGlyphs, so the caller can fall back to small text instead
// of drawing gaps for missing glyphs.
func renderBigLyricLine(v *Visualizer, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !bigFontSupports(text) {
		return "", false
	}
	upper := strings.ToUpper(text)

	dotRows := v.Rows * 4
	dotCols := PanelWidth * 2
	if dotRows <= 0 || dotCols <= 0 {
		return "", false
	}

	maxScale := min(bigFontMaxScale, dotCols/(bigFontW+bigFontGap))
	for scale := maxScale; scale >= 1; scale-- {
		charsPerRow := dotCols / ((bigFontW + bigFontGap) * scale)
		if charsPerRow < 1 {
			continue
		}
		rows := wrapBigText(upper, charsPerRow)
		if len(rows) == 0 || len(rows) > bigFontMaxLines {
			continue
		}
		rowHeight := (bigFontH + bigFontLineGap) * scale
		if rowHeight*len(rows) > dotRows {
			continue
		}
		return drawBigTextRows(v, rows, scale, dotRows, dotCols), true
	}
	return "", false
}

// wrapBigText greedily wraps already-upper-cased text to charsPerRow,
// breaking on spaces. A single word wider than a whole row is hard-cut —
// rare, and better than refusing to render the line at all.
func wrapBigText(text string, charsPerRow int) []string {
	words := strings.Fields(text)
	if len(words) == 0 || charsPerRow < 1 {
		return nil
	}

	var rows []string
	var cur []rune
	for _, w := range words {
		word := []rune(w)
		for len(word) > charsPerRow {
			if len(cur) > 0 {
				rows = append(rows, string(cur))
				cur = nil
			}
			rows = append(rows, string(word[:charsPerRow]))
			word = word[charsPerRow:]
		}
		need := len(word)
		if len(cur) > 0 {
			need++ // separating space
		}
		if len(cur)+need > charsPerRow {
			rows = append(rows, string(cur))
			cur = nil
		}
		if len(cur) > 0 {
			cur = append(cur, ' ')
		}
		cur = append(cur, word...)
	}
	if len(cur) > 0 {
		rows = append(rows, string(cur))
	}
	return rows
}

// drawBigTextRows stamps each row of text into a dot grid at scale, centres
// the whole block vertically and each row individually horizontally, then
// converts to Braille exactly as renderLogo does.
func drawBigTextRows(v *Visualizer, rows []string, scale, dotRows, dotCols int) string {
	grid := make([]bool, dotRows*dotCols)
	rowHeight := (bigFontH + bigFontLineGap) * scale
	top := (dotRows - rowHeight*len(rows)) / 2

	for ri, row := range rows {
		runes := []rune(row)
		lineWidth := len(runes)*(bigFontW+bigFontGap)*scale - bigFontGap*scale
		x := (dotCols - lineWidth) / 2
		y := top + ri*rowHeight

		for _, r := range runes {
			if glyph, ok := bigFontGlyphs[r]; ok {
				stampGlyph(grid, dotCols, dotRows, glyph, x, y, scale)
			}
			x += (bigFontW + bigFontGap) * scale
		}
	}

	lines := make([]string, v.Rows)
	for row := range v.Rows {
		var content strings.Builder
		for ch := range PanelWidth {
			var braille rune = '⠀'
			for dr := range 4 {
				for dc := range 2 {
					if grid[(row*4+dr)*dotCols+ch*2+dc] {
						braille |= brailleBit[dr][dc]
					}
				}
			}
			content.WriteRune(braille)
		}
		lines[row] = lyricsCurrentStyle.Render(content.String())
	}
	return strings.Join(lines, "\n")
}

// stampGlyph sets the dots for one scaled character at (x, y) in grid,
// clipping anything that lands outside it.
func stampGlyph(grid []bool, dotCols, dotRows int, glyph [bigFontH]uint8, x, y, scale int) {
	for py := range bigFontH {
		bits := glyph[py]
		for px := range bigFontW {
			if bits&(1<<(bigFontW-1-px)) == 0 {
				continue
			}
			for sy := range scale {
				for sx := range scale {
					dx, dy := x+px*scale+sx, y+py*scale+sy
					if dx < 0 || dx >= dotCols || dy < 0 || dy >= dotRows {
						continue
					}
					grid[dy*dotCols+dx] = true
				}
			}
		}
	}
}
