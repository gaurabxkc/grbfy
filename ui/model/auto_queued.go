package model

import (
	"maps"

	"github.com/bjarneo/cliamp/playlist"
)

// markAutoQueued tags a track as queued by a plugin rather than chosen by the
// listener. Everything arriving through the track.queue IPC op is a program
// acting on its own — autoplay's Last.fm suggestions, in practice — while the
// q and a keys go straight to Playlist.Queue and stay unmarked.
//
// PlayNow reads the tag: starting a new radio session keeps what you queued
// and discards generated filler, so a second search doesn't inherit the first
// session's leftovers (and doesn't leave the queue so full that autoplay
// concludes it has nothing to do).
func markAutoQueued(track playlist.Track) playlist.Track {
	// Copy before writing: the map can still be shared with the request that
	// carried the track in, and tagging that would leak into its next use.
	meta := maps.Clone(track.ProviderMeta)
	if meta == nil {
		meta = map[string]string{}
	}
	meta[playlist.MetaAutoQueued] = "1"
	track.ProviderMeta = meta
	return track
}
