package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

// maxConsecutiveSkips bounds how many unplayable tracks in a row are skipped
// before giving up. Skipping recurses (skip -> play next -> fails -> skip), so
// something has to stop it. A counter does that without marking tracks, which
// matters because the provider's "unavailable" answer is not always the truth:
// the AES key request also fails under transient load, and permanently
// flagging a track on one refusal mislabels perfectly playable ones.
const maxConsecutiveSkips = 4

// skipUnavailableTrack advances past a track the provider refused to stream
// even though the session is healthy — a region lock, a pulled catalogue
// entry, or an ID the account has no rights to.
//
// The track is deliberately NOT marked Unplayable: that is a permanent,
// visible label ("(unavailable)" in the list) and a single transient refusal
// should not earn it. Playback simply moves on, and the track is playable
// again on the next attempt if the failure was temporary.
func (m *Model) skipUnavailableTrack(track playlist.Track) tea.Cmd {
	m.consecutiveSkips++
	if m.consecutiveSkips > maxConsecutiveSkips {
		m.consecutiveSkips = 0
		m.player.Stop()
		m.status.Warningf(statusTTLDefault,
			"Stopped: %d tracks in a row could not be played", maxConsecutiveSkips)
		return nil
	}

	m.err = nil
	m.status.Showf(statusTTLShort, "Unavailable, skipping: %s", track.DisplayName())
	return m.nextTrack()
}

// noteTrackPlayed clears the consecutive-skip guard once something actually
// plays, so an unrelated bad patch later doesn't start from a primed counter.
func (m *Model) noteTrackPlayed() {
	m.consecutiveSkips = 0
}
