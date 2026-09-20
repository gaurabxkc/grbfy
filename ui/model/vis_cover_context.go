package model

import "github.com/bjarneo/cliamp/ui"

// visualizerCoverContext resolves the artwork for the Cover visualizer. It is
// the only place that couples the visualizer to playlist state; the driver
// renders what this hands it.
func (m *Model) visualizerCoverContext() ui.VisCoverContext {
	track, idx := m.currentPlaybackTrack()
	if idx < 0 {
		return ui.VisCoverContext{}
	}
	return ui.VisCoverContext{
		ArtURL:      track.AlbumArtURL,
		TrackTitle:  track.Title,
		TrackArtist: track.Artist,
	}
}
