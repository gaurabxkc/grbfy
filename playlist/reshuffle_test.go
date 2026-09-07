package playlist

import (
	"slices"
	"testing"
)

func TestReshuffleKeepsCurrentTrack(t *testing.T) {
	p := makePlaylist(10, true)
	p.Next() // move off the first slot

	before, beforeIdx := p.Current()

	if !p.Reshuffle() {
		t.Fatal("Reshuffle() = false, want true while shuffle is on")
	}

	after, afterIdx := p.Current()
	if after.Title != before.Title || afterIdx != beforeIdx {
		t.Fatalf("current track changed: %q(%d) -> %q(%d)", before.Title, beforeIdx, after.Title, afterIdx)
	}
}

func TestReshuffleDrawsNewOrder(t *testing.T) {
	// With 30 tracks an identical redraw is vanishingly unlikely, so a stable
	// order across several attempts means the order was not redrawn at all.
	p := makePlaylist(30, true)
	original := slices.Clone(p.order)

	changed := false
	for range 5 {
		p.Reshuffle()
		if !slices.Equal(p.order, original) {
			changed = true
			break
		}
	}
	if !changed {
		t.Error("Reshuffle() never changed the order")
	}
}

func TestReshuffleNoOpWhenShuffleOff(t *testing.T) {
	p := makePlaylist(10, false)
	before := slices.Clone(p.order)
	rev := p.Revision()

	if p.Reshuffle() {
		t.Error("Reshuffle() = true with shuffle off, want false")
	}
	if !slices.Equal(p.order, before) {
		t.Error("Reshuffle() changed the order with shuffle off")
	}
	if got := p.Revision(); got != rev {
		t.Errorf("Revision changed from %d to %d on a no-op", rev, got)
	}
}

func TestReshuffleEmptyPlaylist(t *testing.T) {
	p := New()
	if p.Reshuffle() {
		t.Error("Reshuffle() = true on an empty playlist, want false")
	}
}

func TestReshuffleBumpsRevision(t *testing.T) {
	p := makePlaylist(10, true)
	rev := p.Revision()

	p.Reshuffle()

	if got := p.Revision(); got <= rev {
		t.Errorf("Revision = %d, want greater than %d so the UI redraws", got, rev)
	}
}

// The re-rolled order must be a real permutation: every track exactly once.
func TestReshuffleKeepsEveryTrack(t *testing.T) {
	p := makePlaylist(12, true)
	p.Reshuffle()

	seen := make(map[int]bool, 12)
	for _, idx := range p.order {
		if seen[idx] {
			t.Fatalf("track index %d appears twice in the order", idx)
		}
		seen[idx] = true
	}
	if len(seen) != 12 {
		t.Fatalf("order covers %d tracks, want 12", len(seen))
	}
}
