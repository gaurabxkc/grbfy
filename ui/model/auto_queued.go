package model

import (
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
	if track.ProviderMeta == nil {
		track.ProviderMeta = map[string]string{}
	}
	track.ProviderMeta[playlist.MetaAutoQueued] = "1"
	return track
}
