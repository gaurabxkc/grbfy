package playlist

import "testing"

func TestActivateQueuedTrackDropsEarlierQueueEntries(t *testing.T) {
	p := makePlaylist(7, false) // A..G
	// Queue B..G, mirroring an autoplay-filled radio queue.
	for i := 1; i <= 6; i++ {
		p.Queue(i)
	}
	if n := p.QueueLen(); n != 6 {
		t.Fatalf("setup: QueueLen() = %d, want 6", n)
	}

	// Jump to the last queued track (G): everything queued before it was
	// skipped on purpose and must not play afterwards.
	p.SetIndex(6)
	act, ok := p.ActivateSelected()
	if !ok || act.Track.Title != "G" {
		t.Fatalf("ActivateSelected() = %q, %v; want G, true", act.Track.Title, ok)
	}
	if n := p.QueueLen(); n != 0 {
		t.Errorf("QueueLen() = %d after jumping past the queue, want 0", n)
	}
	if _, ok := p.Next(); ok {
		t.Error("Next() should find nothing: the skipped entries are gone")
	}
}

func TestActivateUnqueuedTrackLeavesQueueAlone(t *testing.T) {
	p := makePlaylist(5, false) // A..E
	p.Queue(3)                  // D queued to play next

	// Activating a track that isn't queued is not a decision about the
	// queue, so it still plays afterwards.
	p.SetIndex(1)
	if _, ok := p.ActivateSelected(); !ok {
		t.Fatal("ActivateSelected() failed")
	}
	if n := p.QueueLen(); n != 1 {
		t.Fatalf("QueueLen() = %d, want the queue untouched", n)
	}
	track, ok := p.Next()
	if !ok || track.Title != "D" {
		t.Errorf("Next() = %q, %v; want the queued D", track.Title, ok)
	}
}
