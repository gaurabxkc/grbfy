package model

import (
	"fmt"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
	"github.com/bjarneo/cliamp/ui"
)

// visualizerCoverContext resolves the artwork for the Cover visualizer. It is
// the only place that couples the visualizer to playlist state; the driver
// renders what this hands it.
func (m *Model) visualizerCoverContext() ui.VisCoverContext {
	track, idx := m.currentPlaybackTrack()
	if idx < 0 {
		return ui.VisCoverContext{}
	}
	// The large artwork when the provider carries one: AlbumArtURL is a
	// thumbnail, and a cover at panel size wants the full-resolution image.
	art := track.AlbumArtURL
	if large := track.Meta(provider.MetaAlbumArtLarge); large != "" {
		art = large
	}
	album := track.Album
	if album != "" && track.Year != 0 {
		album = fmt.Sprintf("%s · %d", album, track.Year)
	}
	return ui.VisCoverContext{
		ArtURL:       art,
		TrackTitle:   track.Title,
		TrackArtist:  track.Artist,
		AlbumLine:    album,
		PositionSecs: int(m.cachedPos.Seconds()),
		DurationSecs: coverDuration(m, track),
		NextLine:     m.coverNextLine(),
	}
}

// coverDuration is the playing track's length. A live stream reports none,
// and the cached duration is preferred because it follows the player rather
// than the playlist's metadata.
func coverDuration(m *Model, track playlist.Track) int {
	if m.currentPlaybackIsLive(track) {
		return 0
	}
	if secs := int(m.cachedDur.Seconds()); secs > 0 {
		return secs
	}
	return track.DurationSecs
}

// coverNextLine names what plays after this track, resolved the same way Up
// Next resolves it, so the screen agrees with the U overlay.
func (m *Model) coverNextLine() string {
	if m.playlist == nil {
		return ""
	}
	entries, _ := m.playlist.UpcomingWindow(1)
	if len(entries) == 0 {
		return ""
	}
	return trackViewName(entries[0].Track)
}
