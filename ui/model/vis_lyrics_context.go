package model

import (
	"github.com/bjarneo/cliamp/lyrics"
	"github.com/bjarneo/cliamp/ui"
)

// visualizerLyricsContext resolves what the Lyrics visualizer should draw for
// the current playback position. It is the only place that couples the
// visualizer to lyrics and player state; the driver itself just renders what
// this hands it.
//
// The syncability rules are the overlay's, reused rather than restated: a
// track that cannot show synced lyrics in the y panel cannot show them here
// either.
func (m *Model) visualizerLyricsContext() ui.VisLyricsContext {
	artist, title := m.lyricsArtistTitle()
	ctx := ui.VisLyricsContext{
		Syncable:    m.lyricsSyncable(),
		HasLines:    len(m.lyrics.lines) > 0 && m.lyricsHaveTimestamps(),
		TrackTitle:  title,
		TrackArtist: artist,
	}
	if !ctx.Syncable || !ctx.HasLines || m.player == nil {
		return ctx
	}

	// Offset-adjusted, so [ and ] retime the visualizer along with the overlay.
	idx := lyrics.ActiveLineIndex(m.lyrics.lines, m.lyricsPlaybackPosition())
	if idx < 0 {
		// Before the first line: the caller falls back to track info.
		return ctx
	}
	ctx.CurrentLine = m.lyrics.lines[idx].Text
	if ctx.CurrentLine == "" {
		ctx.CurrentLine = "♪"
	}
	if idx+1 < len(m.lyrics.lines) {
		ctx.NextLine = m.lyrics.lines[idx+1].Text
	}
	return ctx
}
