package playlist

import "testing"

// titlesOf reduces a window to titles so failures read as the order a listener
// would see.
func titlesOf(entries []UpcomingEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Track.Title
	}
	return out
}

// playedTitles advances the playlist n times and records what actually played.
func playedTitles(t *testing.T, p *Playlist, n int) []string {
	t.Helper()
	out := make([]string, 0, n)
	for range n {
		track, ok := p.Next()
		if !ok {
			break
		}
		out = append(out, track.Title)
	}
	return out
}

func equalTitles(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The whole point of the feature: what the panel predicts must be what plays.
func TestUpcomingWindowMatchesActualPlayback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		shuffle bool
		repeat  RepeatMode
	}{
		{"sequential", false, RepeatOff},
		{"shuffled", true, RepeatOff},
		{"sequential repeat all", false, RepeatAll},
		{"shuffled repeat all", true, RepeatAll},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := makePlaylist(8, tc.shuffle)
			p.SetRepeat(tc.repeat)

			predicted, _ := p.UpcomingWindow(5)
			want := titlesOf(predicted)
			got := playedTitles(t, p, len(want))

			if !equalTitles(got, want) {
				t.Fatalf("played %v, but the panel predicted %v", got, want)
			}
		})
	}
}

func TestUpcomingWindowListsQueueFirst(t *testing.T) {
	p := makePlaylist(6, false) // A..F, currently A

	p.Queue(4) // E
	p.Queue(2) // C

	// Playing from the queue does not advance the order position, so a queued
	// track plays early and still comes round again in its natural slot: C is
	// listed twice on purpose.
	entries, _ := p.UpcomingWindow(4)
	want := []string{"E", "C", "B", "C"}
	if got := titlesOf(entries); !equalTitles(got, want) {
		t.Fatalf("UpcomingWindow = %v, want %v", got, want)
	}

	if !entries[0].Queued || !entries[1].Queued {
		t.Error("queued entries should be marked Queued")
	}
	if entries[2].Queued {
		t.Error("order entries should not be marked Queued")
	}

	if got := playedTitles(t, p, 4); !equalTitles(got, want) {
		t.Fatalf("played %v, want %v", got, want)
	}
}

// With shuffle + RepeatAll the wrap draws a fresh order, so the window must
// stop and say so rather than inventing an order.
func TestUpcomingWindowReportsReshuffleAtWrap(t *testing.T) {
	p := makePlaylist(3, true)
	p.SetRepeat(RepeatAll)

	// Move to the last slot of the order so the next advance wraps.
	p.Next()
	p.Next()

	entries, reshuffles := p.UpcomingWindow(10)
	if !reshuffles {
		t.Fatal("reshuffles = false at a shuffle wrap, want true")
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %v, want none past the wrap", titlesOf(entries))
	}

	// Sequential RepeatAll wraps to a known order, so it must NOT report a reshuffle.
	q := makePlaylist(3, false)
	q.SetRepeat(RepeatAll)
	q.Next()
	q.Next()

	entries, reshuffles = q.UpcomingWindow(10)
	if reshuffles {
		t.Error("sequential wrap should not report a reshuffle")
	}
	if got, want := titlesOf(entries), []string{"A", "B", "C"}; !equalTitles(got, want) {
		t.Fatalf("sequential wrap = %v, want %v", got, want)
	}
}

func TestUpcomingWindowSkipsUnplayable(t *testing.T) {
	p := makePlaylist(5, false) // A..E
	p.SetTrack(1, Track{Title: "B", Unplayable: true})
	p.SetTrack(2, Track{Title: "C", Unplayable: true})

	entries, _ := p.UpcomingWindow(3)
	want := []string{"D", "E"}
	if got := titlesOf(entries); !equalTitles(got, want) {
		t.Fatalf("UpcomingWindow = %v, want %v (unplayable tracks must be skipped)", got, want)
	}
	if got := playedTitles(t, p, 2); !equalTitles(got, want) {
		t.Fatalf("played %v, want %v", got, want)
	}
}

func TestUpcomingWindowRepeatOne(t *testing.T) {
	p := makePlaylist(4, false)
	p.SetIndex(1) // B
	p.SetRepeat(RepeatOne)

	entries, reshuffles := p.UpcomingWindow(5)
	if reshuffles {
		t.Error("RepeatOne should not report a reshuffle")
	}
	if got, want := titlesOf(entries), []string{"B"}; !equalTitles(got, want) {
		t.Fatalf("UpcomingWindow = %v, want %v", got, want)
	}
}

func TestUpcomingWindowRespectsLimit(t *testing.T) {
	p := makePlaylist(20, false)

	entries, _ := p.UpcomingWindow(3)
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	for _, limit := range []int{0, -1} {
		if entries, _ := p.UpcomingWindow(limit); len(entries) != 0 {
			t.Errorf("UpcomingWindow(%d) returned %d entries, want 0", limit, len(entries))
		}
	}
}

func TestUpcomingWindowEmptyPlaylist(t *testing.T) {
	p := New()
	entries, reshuffles := p.UpcomingWindow(5)
	if len(entries) != 0 || reshuffles {
		t.Fatalf("empty playlist = (%d entries, reshuffles=%v), want (0, false)", len(entries), reshuffles)
	}
}

// The window is a read-only view; listing must not disturb playback.
func TestUpcomingWindowDoesNotMutate(t *testing.T) {
	p := makePlaylist(6, true)
	p.SetRepeat(RepeatAll)

	before := p.Snapshot()
	revBefore := p.Revision()

	for range 5 {
		p.UpcomingWindow(50)
	}

	if !p.matches(before) {
		t.Error("UpcomingWindow mutated playback state")
	}
	if got := p.Revision(); got != revBefore {
		t.Errorf("Revision changed from %d to %d", revBefore, got)
	}
}

// A returned Track must be a copy: mutating it cannot reach the playlist.
func TestUpcomingWindowReturnsCopies(t *testing.T) {
	p := makePlaylist(3, false)

	entries, _ := p.UpcomingWindow(1)
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	entries[0].Track.Title = "mutated"

	if track, ok := p.Track(entries[0].TrackIndex); !ok || track.Title == "mutated" {
		t.Error("UpcomingWindow leaked a reference into the playlist")
	}
}

func TestJumpToUpcomingQueued(t *testing.T) {
	p := makePlaylist(6, false) // A..F, currently A
	p.Queue(4)                  // E
	p.Queue(2)                  // C

	// Jumping to the second queued entry skips the first.
	track, ok := p.JumpToUpcoming(1, 50)
	if !ok || track.Title != "C" {
		t.Fatalf("JumpToUpcoming(1) = %q,%v, want C,true", track.Title, ok)
	}
	if cur, _ := p.Current(); cur.Title != "C" {
		t.Errorf("current = %q, want C", cur.Title)
	}
	if got := p.QueueLen(); got != 0 {
		t.Errorf("queue length = %d, want 0: the skipped entry should be gone", got)
	}
}

func TestJumpToUpcomingFromOrderKeepsQueue(t *testing.T) {
	p := makePlaylist(6, false) // A..F, currently A
	p.Queue(5)                  // F stays queued

	// Entry 0 is queued F; entry 1 is B from the order.
	track, ok := p.JumpToUpcoming(1, 50)
	if !ok || track.Title != "B" {
		t.Fatalf("JumpToUpcoming(1) = %q,%v, want B,true", track.Title, ok)
	}
	if got := p.QueueLen(); got != 1 {
		t.Errorf("queue length = %d, want 1: jumping within the order must not drop the queue", got)
	}
}

func TestRemoveUpcomingOnlyQueued(t *testing.T) {
	p := makePlaylist(6, false)
	p.Queue(4) // E

	if !p.RemoveUpcoming(0, 50) {
		t.Fatal("RemoveUpcoming on a queued entry returned false")
	}
	if got := p.QueueLen(); got != 0 {
		t.Errorf("queue length = %d, want 0", got)
	}
	// An order entry is not the queue's to drop: the caller removes the track
	// itself, which keeps the order a permutation of the playlist.
	if p.RemoveUpcoming(0, 50) {
		t.Error("RemoveUpcoming removed an order entry, which would desync the order")
	}
	if p.Len() != 6 {
		t.Errorf("playlist length = %d, want 6 untouched", p.Len())
	}
}

func TestMoveUpcomingWithinQueue(t *testing.T) {
	p := makePlaylist(6, false)
	p.Queue(4) // E
	p.Queue(2) // C

	before, _ := p.UpcomingWindow(2)
	if before[0].Track.Title != "E" || before[1].Track.Title != "C" {
		t.Fatalf("setup = %v", titlesOf(before))
	}

	if got, ok := p.MoveUpcoming(0, 1, 50); !ok || got != 1 {
		t.Fatalf("MoveUpcoming(0,+1) = %d,%v, want 1,true", got, ok)
	}
	after, _ := p.UpcomingWindow(2)
	if after[0].Track.Title != "C" || after[1].Track.Title != "E" {
		t.Errorf("after move = %v, want [C E]", titlesOf(after))
	}
	if got := playedTitles(t, p, 2); !equalTitles(got, []string{"C", "E"}) {
		t.Errorf("played %v, want [C E]", got)
	}
}

func TestMoveUpcomingWithinOrder(t *testing.T) {
	p := makePlaylist(6, false) // A..F, upcoming B C D E F

	if _, ok := p.MoveUpcoming(0, 1, 50); !ok {
		t.Fatal("MoveUpcoming within the order returned false")
	}
	after, _ := p.UpcomingWindow(2)
	if after[0].Track.Title != "C" || after[1].Track.Title != "B" {
		t.Errorf("after move = %v, want [C B]", titlesOf(after))
	}
	if got := playedTitles(t, p, 2); !equalTitles(got, []string{"C", "B"}) {
		t.Errorf("played %v, want [C B]: the order itself must change", got)
	}
}

// A queued entry and an order entry live in different lists; swapping them
// would silently change whether a track is queued.
func TestMoveUpcomingRefusesAcrossSources(t *testing.T) {
	p := makePlaylist(6, false)
	p.Queue(4) // E queued, then order entries follow

	if _, ok := p.MoveUpcoming(0, 1, 50); ok {
		t.Error("MoveUpcoming swapped a queued entry with an order entry")
	}
}

func TestMoveUpcomingBounds(t *testing.T) {
	p := makePlaylist(4, false)
	for _, tc := range []struct{ n, delta int }{{0, -1}, {-1, 1}, {99, 1}, {0, 0}} {
		if _, ok := p.MoveUpcoming(tc.n, tc.delta, 50); ok {
			t.Errorf("MoveUpcoming(%d,%d) = true, want false", tc.n, tc.delta)
		}
	}
}
