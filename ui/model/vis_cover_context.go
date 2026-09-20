package model

import (
	"fmt"

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
		ArtURL:      art,
		TrackTitle:  track.Title,
		TrackArtist: track.Artist,
		AlbumLine:   album,
	}
}
