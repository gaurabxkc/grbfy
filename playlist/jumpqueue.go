package playlist

import "slices"

// dropQueueThrough removes queued entries up to and including the first entry
// for trackIdx, and reports whether trackIdx was queued at all.
//
// Jumping onto a queued track is a decision to skip whatever was queued ahead
// of it, so those entries go rather than lingering to play afterwards.
// JumpToUpcoming already works this way for the Up Next panel; this is the
// same rule for activating a track from the playlist itself. p.mu must be held.
func (p *Playlist) dropQueueThrough(trackIdx int) bool {
	for i, idx := range p.queue {
		if idx == trackIdx {
			p.queue = slices.Delete(p.queue, 0, i+1)
			p.rebuildQueuePositions()
			return true
		}
	}
	return false
}
