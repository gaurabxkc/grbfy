package playlist

// PlayNow starts playing track immediately, discarding the rest of the
// playlist's resolved play order. Tracks already queued with Queue (the "q"
// key) are kept and still play right after it — only order's not-yet-played
// entries are dropped.
//
// This exists for "play now" actions that pick a track from outside the
// currently loaded playlist (search results, in particular): Add followed by
// SetIndex leaves the old playlist's remaining order entries in place, so
// once the searched track ends, playback falls back into whatever the old
// playlist would have played next — order still resolves to the next slot
// after the newly appended one. PlayNow instead starts a fresh session
// containing just the new track plus anything already queued, so nothing
// from the old playlist plays automatically again.
func (p *Playlist) PlayNow(track Track) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Keep what the listener queued; drop what a plugin queued for them.
	// Autoplay's suggestions share the play-next queue with manual q picks,
	// so carrying everything over meant a second search inherited the first
	// session's filler — which also kept the queue long enough that autoplay
	// decided it had nothing to do and never topped up again.
	var queued []Track
	for _, t := range p.cloneQueuedTracks(p.queue) {
		if t.Meta(MetaAutoQueued) == "" {
			queued = append(queued, t)
		}
	}
	p.tracks = append([]Track{cloneTrack(track)}, queued...)

	// order holds only the new track. Queued tracks are reachable through
	// queue, not order — Next checks the queue first, so listing them in
	// both would let them play twice: once popped from queue, then again
	// when advanceFromOrder later walked past this position.
	p.order = []int{0}
	p.pos = 0

	if len(queued) == 0 {
		p.queue = nil
		p.queuePositions = nil
	} else {
		p.queue = make([]int, len(queued))
		for i := range queued {
			p.queue[i] = i + 1
		}
		p.rebuildQueuePositions()
	}
	p.queuedIdx = -1
	// This is a radio context: one deliberately chosen track with nothing
	// queued behind it. Autoplay-style plugins use Radio to decide whether
	// generating a follow-on queue is wanted — inside a loaded playlist the
	// list itself is what plays next, and suggestions are just noise.
	p.radio = true
	p.rebuildBookmarkCount()
	p.revision++
}

// Radio reports whether playback is an ad-hoc single track (PlayNow) rather
// than a loaded playlist. Replace clears it.
func (p *Playlist) Radio() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.radio
}
