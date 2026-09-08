package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func lyricsDriverFor(t *testing.T, v *Visualizer) *lyricsDriver {
	t.Helper()
	driver, ok := activateMode(t, v, VisLyrics).(*lyricsDriver)
	if !ok || driver == nil {
		t.Fatal("activateMode(VisLyrics) did not return *lyricsDriver")
	}
	return driver
}

func TestLyricsMode(t *testing.T) {
	mode, ok := StringToVisModeExact("Lyrics")
	if !ok || mode != VisLyrics {
		t.Fatalf("StringToVisModeExact(Lyrics) = (%v, %v), want (%v, true)", mode, ok, VisLyrics)
	}

	v := NewVisualizer(44100)
	activateMode(t, v, VisLyrics)
	if got := v.ModeName(); got != "Lyrics" {
		t.Fatalf("ModeName() = %q, want Lyrics", got)
	}
	// Lyric lines turn over on the order of seconds; nothing animates between
	// them, so this mode must not ask for the animation cadence.
	if got := v.TickInterval(VisTickContext{Playing: true}); got != TickSlow {
		t.Fatalf("TickInterval(playing) = %v, want %v", got, TickSlow)
	}
}

// Fixtures below append an emoji to text that must exercise the small-text
// fallback path: plain Latin words are drawable in the big pixel font (see
// vis_lyrics.go) and would otherwise route through that renderer instead,
// where none of these literal-substring assertions would hold.

func TestLyricsRendersSyncedLineAndNext(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 5
	driver := lyricsDriverFor(t, v)

	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: true, HasLines: true,
		CurrentLine: "we were the kings 🎵", NextLine: "and the queens 🎵",
		TrackTitle: "Some Song", TrackArtist: "Some Band",
	}})

	got := ansi.Strip(driver.Render(v))
	if !strings.Contains(got, "we were the kings") {
		t.Errorf("render missing the active line:\n%s", got)
	}
	if !strings.Contains(got, "and the queens") {
		t.Errorf("render missing the next line:\n%s", got)
	}
	if strings.Contains(got, "Some Song") {
		t.Errorf("track info should not appear while a synced line is showing:\n%s", got)
	}
}

func TestLyricsFallsBackToTrackInfo(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 5
	driver := lyricsDriverFor(t, v)

	// A synced line first, then a track that can't be synced at all (a radio
	// stream): the earlier line must not survive the switch.
	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: true, HasLines: true, CurrentLine: "stale line",
	}})
	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: false, TrackTitle: "Night Drive 🎵", TrackArtist: "Station FM",
	}})

	got := ansi.Strip(driver.Render(v))
	if strings.Contains(got, "stale line") {
		t.Errorf("previous track's lyric line leaked into the fallback:\n%s", got)
	}
	if !strings.Contains(got, "Night Drive") || !strings.Contains(got, "Station FM") {
		t.Errorf("fallback should show title and artist:\n%s", got)
	}
}

func TestLyricsFallsBackBeforeFirstLine(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 5
	driver := lyricsDriverFor(t, v)

	// Synced lyrics exist, but playback is still in the intro.
	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: true, HasLines: true, CurrentLine: "",
		TrackTitle: "Intro Heavy 🎵", TrackArtist: "Band",
	}})

	got := ansi.Strip(driver.Render(v))
	if !strings.Contains(got, "Intro Heavy") {
		t.Errorf("an instrumental intro should show track info, not a blank panel:\n%s", got)
	}
}

func TestLyricsRendersNoteWithNoMetadataAtAll(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 5
	driver := lyricsDriverFor(t, v)

	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{}})

	if got := ansi.Strip(driver.Render(v)); !strings.Contains(got, lyricsNoteGlyph) {
		t.Errorf("with nothing to show the panel should still draw %q:\n%s", lyricsNoteGlyph, got)
	}
}

func TestLyricsRenderFitsPanel(t *testing.T) {
	withPanelWidth(t, 20)
	v := NewVisualizer(44100)
	v.Rows = 5
	driver := lyricsDriverFor(t, v)

	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: true, HasLines: true,
		CurrentLine: strings.Repeat("long ", 40),
	}})

	for i, line := range strings.Split(ansi.Strip(driver.Render(v)), "\n") {
		if w := ansi.StringWidth(line); w > PanelWidth {
			t.Errorf("line %d is %d cells wide, panel is %d", i, w, PanelWidth)
		}
	}
}

func TestBigFontSupports(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"HELLO WORLD", true},
		{"don't stop, believing!", true}, // lowercase + supported punctuation
		{"", true},                       // vacuously: no unsupported rune
		{"café", false},                  // accented rune has no glyph
		{"नमस्ते", false},                // Devanagari: no hand-authored glyphs at all
		{"🎵", false},                     // emoji
	}
	for _, tc := range cases {
		if got := bigFontSupports(tc.text); got != tc.want {
			t.Errorf("bigFontSupports(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestRenderBigLyricLineRendersSupportedText(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 6

	got, ok := renderBigLyricLine(v, "hello")
	if !ok {
		t.Fatal("renderBigLyricLine() = false for plain Latin text")
	}
	if strings.TrimSpace(ansi.Strip(got)) == "" {
		t.Error("big render produced no visible dots")
	}
	if lines := strings.Split(got, "\n"); len(lines) != v.Rows {
		t.Errorf("got %d rendered rows, want v.Rows = %d", len(lines), v.Rows)
	}

	// Deterministic: rendering the same text twice must not depend on
	// anything but the text and panel size.
	again, ok := renderBigLyricLine(v, "hello")
	if !ok || again != got {
		t.Error("renderBigLyricLine() is not deterministic for identical input")
	}
}

func TestRenderBigLyricLineRejectsUnsupportedText(t *testing.T) {
	v := NewVisualizer(44100)
	v.Rows = 6
	if _, ok := renderBigLyricLine(v, "नमस्ते"); ok {
		t.Error("renderBigLyricLine() should refuse text outside bigFontGlyphs")
	}
	if _, ok := renderBigLyricLine(v, "   "); ok {
		t.Error("renderBigLyricLine() should refuse blank text")
	}
}

func TestRenderBigLyricLineWrapsLongText(t *testing.T) {
	withPanelWidth(t, 30)
	v := NewVisualizer(44100)
	v.Rows = 10

	// Long enough to need more than one row at any scale, short enough to
	// still fit within bigFontMaxLines for this panel size.
	long := strings.Repeat("WORD ", 8)
	got, ok := renderBigLyricLine(v, long)
	if !ok {
		t.Fatal("renderBigLyricLine() = false for a long but fully supported line")
	}
	for i, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > PanelWidth {
			t.Errorf("row %d is %d cells wide, panel is %d", i, w, PanelWidth)
		}
	}

	// A line too long to fit within bigFontMaxLines rows at any scale must
	// be refused, not silently clipped mid-word.
	if _, ok := renderBigLyricLine(v, strings.Repeat("WORD ", 60)); ok {
		t.Error("renderBigLyricLine() should refuse text that can't fit within bigFontMaxLines")
	}
}

func TestLyricsUsesBigFontForPlainLatinLine(t *testing.T) {
	withPanelWidth(t, 40)
	v := NewVisualizer(44100)
	v.Rows = 6
	driver := lyricsDriverFor(t, v)

	driver.Tick(v, VisTickContext{Lyrics: VisLyricsContext{
		Syncable: true, HasLines: true, CurrentLine: "hello",
	}})

	got := driver.Render(v)
	want, ok := renderBigLyricLine(v, "hello")
	if !ok {
		t.Fatal("renderBigLyricLine() unexpectedly refused \"hello\"")
	}
	if got != want {
		t.Error("driver did not route a plain Latin line through the big-font renderer")
	}
}
