package playlist

import "slices"

// UpcomingEntry is one track in the resolved playback order, ahead of the
// current track.
type UpcomingEntry struct {
	TrackIndex int
	Track      Track
	// Queued reports that this entry comes from the play-next queue rather
	// than from the shuffle or sequential order.
	Queued bool
}

// upcomingRef locates an upcoming entry in whichever list it came from, so an
// edit can act on the right one. Positions index p.queue or p.order.
type upcomingRef struct {
	queued   bool
	pos      int
	trackIdx int
}

// upcomingRefs walks the resolved playback order ahead of the current track,
// following the same rules as Next: the play-next queue first, then the order
// from the current position, honoring the repeat mode and skipping unplayable
// tracks.
//
// reshuffles reports that the walk stopped early because the next order is not
// yet decided: with shuffle and RepeatAll, running off the end of the order
// reshuffles it, and that draw has not happened.
//
// The caller must hold p.mu.
func (p *Playlist) upcomingRefs(limit int) (refs []upcomingRef, reshuffles bool) {
	if limit <= 0 || len(p.tracks) == 0 || len(p.order) == 0 {
		return nil, false
	}
	refs = make([]upcomingRef, 0, limit)

	// The play-next queue drains first. Next skips unplayable entries, so a
	// listing that showed them would not match what plays.
	for i, idx := range p.queue {
		if len(refs) == limit {
			return refs, false
		}
		if p.isPlayable(idx) {
			refs = append(refs, upcomingRef{queued: true, pos: i, trackIdx: idx})
		}
	}
	if len(refs) == limit {
		return refs, false
	}

	// RepeatOne replays the current track once the queue drains. Listing it
	// once conveys that; repeating it to fill the window would be noise.
	if p.repeat == RepeatOne {
		if pos := p.pos; pos < len(p.order) {
			if idx := p.order[pos]; p.isPlayable(idx) {
				refs = append(refs, upcomingRef{pos: pos, trackIdx: idx})
			}
		}
		return refs, false
	}

	for i := p.pos + 1; i < len(p.order) && len(refs) < limit; i++ {
		if idx := p.order[i]; p.isPlayable(idx) {
			refs = append(refs, upcomingRef{pos: i, trackIdx: idx})
		}
	}
	if len(refs) == limit || p.repeat != RepeatAll {
		return refs, false
	}

	// RepeatAll wraps to the start. Under shuffle that wrap draws a new order
	// (see nextShuffleWrap), so nothing past this point is known yet.
	if p.shuffle {
		return refs, true
	}
	for i := 0; i <= p.pos && i < len(p.order) && len(refs) < limit; i++ {
		if idx := p.order[i]; p.isPlayable(idx) {
			refs = append(refs, upcomingRef{pos: i, trackIdx: idx})
		}
	}
	return refs, false
}

// UpcomingWindow returns at most limit tracks in the order they will actually
// play. See upcomingRefs for the rules and for what reshuffles means.
//
// This never mutates playback state, unlike the advanceFromOrder path Next
// uses, which reshuffles in place on wrap.
func (p *Playlist) UpcomingWindow(limit int) (entries []UpcomingEntry, reshuffles bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	refs, reshuffles := p.upcomingRefs(limit)
	entries = make([]UpcomingEntry, 0, len(refs))
	for _, r := range refs {
		entries = append(entries, UpcomingEntry{
			TrackIndex: r.trackIdx,
			Track:      cloneTrack(p.tracks[r.trackIdx]),
			Queued:     r.queued,
		})
	}
	return entries, reshuffles
}

// UpcomingLen returns how many entries the Up Next list currently holds, up to
// limit.
func (p *Playlist) UpcomingLen(limit int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	refs, _ := p.upcomingRefs(limit)
	return len(refs)
}

// JumpToUpcoming makes the nth upcoming entry the current track and returns it.
//
// Choosing a queued entry discards the queued entries before it: picking one
// past them is a decision to skip them. Choosing an entry from the order moves
// the position there and leaves the queue alone, so it still plays afterwards.
func (p *Playlist) JumpToUpcoming(n, limit int) (Track, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	refs, _ := p.upcomingRefs(limit)
	if n < 0 || n >= len(refs) {
		return Track{}, false
	}
	r := refs[n]

	if r.queued {
		p.queue = slices.Delete(p.queue, 0, r.pos+1)
		p.rebuildQueuePositions()
		p.queuedIdx = r.trackIdx
	} else {
		p.pos = r.pos
		p.queuedIdx = -1
	}
	p.revision++
	return cloneTrack(p.tracks[r.trackIdx]), true
}

// RemoveUpcoming drops the nth upcoming entry, which must be a queued one.
//
// Entries that come from the order are not removable here: the order is a
// permutation of the whole playlist, so dropping one would mean removing the
// track itself. Use the playlist's own remove for that.
func (p *Playlist) RemoveUpcoming(n, limit int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	refs, _ := p.upcomingRefs(limit)
	if n < 0 || n >= len(refs) || !refs[n].queued {
		return false
	}
	p.queue = slices.Delete(p.queue, refs[n].pos, refs[n].pos+1)
	p.rebuildQueuePositions()
	p.revision++
	return true
}

// MoveUpcoming swaps the nth upcoming entry with its neighbour delta places
// away, and reports the entry's new position in the list.
//
// A queued entry and an order entry cannot trade places: they live in different
// lists, and moving one across would silently change whether it is queued.
func (p *Playlist) MoveUpcoming(n, delta, limit int) (newIndex int, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	refs, _ := p.upcomingRefs(limit)
	to := n + delta
	if n < 0 || n >= len(refs) || to < 0 || to >= len(refs) || delta == 0 {
		return n, false
	}
	a, b := refs[n], refs[to]
	if a.queued != b.queued {
		return n, false
	}

	if a.queued {
		p.queue[a.pos], p.queue[b.pos] = p.queue[b.pos], p.queue[a.pos]
		p.rebuildQueuePositions()
	} else {
		p.order[a.pos], p.order[b.pos] = p.order[b.pos], p.order[a.pos]
	}
	p.revision++
	return to, true
}
