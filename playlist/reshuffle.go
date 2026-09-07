package playlist

// Reshuffle draws a new shuffle order without disturbing what is playing. It
// exists so a listener who dislikes the upcoming order can re-roll it directly,
// rather than toggling shuffle off and on again, which is the only way upstream
// offers and which also rewrites the order twice.
//
// The current track stays current: doShuffle pins it at position 0. Reshuffle
// is a no-op when shuffle is off, since there is no shuffle order to redraw.
func (p *Playlist) Reshuffle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.shuffle || len(p.tracks) == 0 || len(p.order) == 0 {
		return false
	}
	p.doShuffle()
	p.revision++
	return true
}
