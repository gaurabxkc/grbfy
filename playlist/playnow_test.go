package playlist

import "testing"

func TestPlayNowDropsRestOfOrder(t *testing.T) {
	p := makePlaylist(5, false) // A B C D E
	p.SetIndex(1)               // playing B, C D E still to come

	p.PlayNow(Track{Title: "Z"})

	if got := titles(p); len(got) != 1 || got[0] != "Z" {
		t.Fatalf("tracks after PlayNow = %v, want [Z]", got)
	}
	if p.Len() != 1 {
		t.Errorf("Len() = %d, want 1", p.Len())
	}
	if p.Index() != 0 {
		t.Errorf("Index() = %d, want 0", p.Index())
	}
	if _, ok := p.Next(); ok {
		t.Error("Next() should find nothing after PlayNow with an empty queue and repeat off")
	}
}

func TestPlayNowKeepsQueuedTracks(t *testing.T) {
	p := makePlaylist(5, false) // A B C D E
	p.SetIndex(0)               // playing A
	p.Queue(2)                  // queue C
	p.Queue(4)                  // queue E

	p.PlayNow(Track{Title: "Z"})

	if got := titles(p); len(got) != 3 || got[0] != "Z" || got[1] != "C" || got[2] != "E" {
		t.Fatalf("tracks after PlayNow = %v, want [Z C E]", got)
	}
	if n := p.QueueLen(); n != 2 {
		t.Fatalf("QueueLen() = %d, want 2", n)
	}

	// Next() must serve the queue (C, then E) before falling back to order,
	// and must not replay either — order holds only the new track, so once
	// the queue drains there is nothing left to advance into.
	track, ok := p.Next()
	if !ok || track.Title != "C" {
		t.Fatalf("first Next() = %q, %v, want C, true", track.Title, ok)
	}
	track, ok = p.Next()
	if !ok || track.Title != "E" {
		t.Fatalf("second Next() = %q, %v, want E, true", track.Title, ok)
	}
	if _, ok := p.Next(); ok {
		t.Error("Next() should find nothing once the queue is drained")
	}
}

func TestPlayNowClearsPriorQueuedIdx(t *testing.T) {
	p := makePlaylist(3, false) // A B C
	p.Queue(1)                  // queue B
	if _, ok := p.Next(); !ok {
		t.Fatal("setup: Next() should serve the queued track")
	}
	if p.queuedIdx == -1 {
		t.Fatal("setup: queuedIdx should be set after playing from the queue")
	}

	p.PlayNow(Track{Title: "Z"})

	if p.queuedIdx != -1 {
		t.Errorf("queuedIdx = %d after PlayNow, want -1", p.queuedIdx)
	}
}

func TestPlayNowOnEmptyPlaylist(t *testing.T) {
	p := New()
	p.PlayNow(Track{Title: "Z"})

	if got := titles(p); len(got) != 1 || got[0] != "Z" {
		t.Fatalf("tracks after PlayNow on an empty playlist = %v, want [Z]", got)
	}
}

func TestPlayNowMarksRadioAndReplaceClearsIt(t *testing.T) {
	p := makePlaylist(5, false)
	if p.Radio() {
		t.Error("a loaded playlist should not be a radio context")
	}

	p.PlayNow(Track{Title: "Z"})
	if !p.Radio() {
		t.Error("PlayNow should start a radio context")
	}

	// Loading a real list ends radio mode, so autoplay stops topping up.
	p.Replace([]Track{{Title: "A"}, {Title: "B"}})
	if p.Radio() {
		t.Error("Replace should clear the radio context")
	}
}

func TestPlayNowDropsAutoQueuedButKeepsChosen(t *testing.T) {
	p := makePlaylist(4, false) // A B C D
	p.Queue(1)                  // B: chosen by the listener with q
	p.Queue(2)                  // C: an autoplay suggestion

	// Mark C the way the track.queue IPC path does.
	c, _ := p.Track(2)
	c.ProviderMeta = map[string]string{MetaAutoQueued: "1"}
	p.SetTrack(2, c)

	p.PlayNow(Track{Title: "Z"})

	got := titles(p)
	if len(got) != 2 || got[0] != "Z" || got[1] != "B" {
		t.Fatalf("tracks after PlayNow = %v, want [Z B] (C was auto-queued filler)", got)
	}
	if n := p.QueueLen(); n != 1 {
		t.Errorf("QueueLen() = %d, want 1 (only the chosen track survives)", n)
	}

	track, ok := p.Next()
	if !ok || track.Title != "B" {
		t.Errorf("Next() = %q, %v; want the chosen track B", track.Title, ok)
	}
}
