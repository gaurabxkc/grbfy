package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

// radioTestProvider builds a station from any track.
type radioTestProvider struct {
	commandsTestProvider
	tracks []playlist.Track
	err    error
}

func (p *radioTestProvider) TrackRadio(context.Context, string) ([]playlist.Track, error) {
	return p.tracks, p.err
}

func radioModel(t *testing.T, prov playlist.Provider) Model {
	t.Helper()
	player := &playbackFakeEngine{}
	m := Model{
		player:   player,
		playlist: playlist.New(),
		provider: prov,
		vis:      ui.NewVisualizer(float64(player.SampleRate())),
	}
	m.focus = focusPlaylist
	m.replacePlaylist([]playlist.Track{{Path: "spotify:track:seed", Title: "Seed"}})
	m.plCursor = 0
	return m
}

func TestTrackRadioReplacesTheQueueWithTheStation(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"},
		tracks: []playlist.Track{{Path: "spotify:track:a"}, {Path: "spotify:track:b"}}}
	m := radioModel(t, prov)

	if cmd := m.startTrackRadio(); cmd == nil {
		t.Fatal("no request was started")
	}
	if !m.trackRadio.starting {
		t.Fatal("the model does not know a station is starting")
	}

	m.applyTrackRadio(trackRadioMsg{seed: playlist.Track{Path: "spotify:track:seed", Title: "Seed"}, tracks: prov.tracks, gen: m.requests.tracks})
	if m.playlist.Len() != 3 {
		t.Fatalf("queue has %d tracks, want the seed and the station's 2", m.playlist.Len())
	}
	if first, _ := m.playlist.Track(0); first.Path != "spotify:track:seed" {
		t.Fatalf("queue starts with %q, want the seed", first.Path)
	}
	if m.trackRadio.starting {
		t.Fatal("still marked as starting after the station arrived")
	}
}

// A station is expensive to start, so a second press inside the window is
// refused rather than queued: each start opens two tracks at once, and
// Spotify stops answering when opens come too fast.
func TestTrackRadioIsRateLimited(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"},
		tracks: []playlist.Track{{Path: "spotify:track:a"}}}
	m := radioModel(t, prov)
	m.trackRadio.lastStart = time.Now()

	if cmd := m.startTrackRadio(); cmd != nil {
		t.Fatal("a second station started inside the interval")
	}
}

// A press while a request is in flight must not start another: a held key
// repeats, and nothing upstream filters that.
func TestTrackRadioIgnoresAPressWhileStarting(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := radioModel(t, prov)
	m.trackRadio.starting = true

	if cmd := m.startTrackRadio(); cmd != nil {
		t.Fatal("a second request started while one was in flight")
	}
}

func TestTrackRadioReportsAProviderThatCannotDoIt(t *testing.T) {
	m := radioModel(t, &commandsTestProvider{name: "Radio"})
	if cmd := m.startTrackRadio(); cmd != nil {
		t.Fatal("a provider without TrackRadio started a station")
	}
}

// An empty station and a failed one both leave the queue alone.
func TestTrackRadioKeepsTheQueueWhenTheStationFails(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := radioModel(t, prov)
	before := m.playlist.Len()

	m.applyTrackRadio(trackRadioMsg{gen: m.requests.tracks, err: errors.New("no station")})
	m.applyTrackRadio(trackRadioMsg{gen: m.requests.tracks})

	if m.playlist.Len() != before {
		t.Fatalf("queue changed to %d tracks after a failed station", m.playlist.Len())
	}
}

// A station that arrives after the user has moved on is dropped.
func TestTrackRadioIgnoresAStaleStation(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := radioModel(t, prov)
	before := m.playlist.Len()

	m.applyTrackRadio(trackRadioMsg{gen: m.requests.tracks + 5, tracks: []playlist.Track{{Path: "spotify:track:late"}}})
	if m.playlist.Len() != before {
		t.Fatal("a stale station replaced the queue")
	}
}

// Radio from the song already playing keeps it playing, first in the queue,
// and the station follows it rather than cutting it off.
func TestTrackRadioLetsThePlayingSeedPlayOn(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := radioModel(t, prov)
	engine := m.player.(*playbackFakeEngine)
	seed, _ := m.playlist.Track(0)
	m.playlist.SetIndex(0)
	m.setPlaybackTrack(seed)
	engine.playing = true
	playsBefore := len(engine.playCalls)

	m.applyTrackRadio(trackRadioMsg{
		seed:   seed,
		tracks: []playlist.Track{{Path: "spotify:track:seed"}, {Path: "spotify:track:a"}, {Path: "spotify:track:b"}},
		gen:    m.requests.tracks,
	})

	if len(engine.playCalls) != playsBefore {
		t.Fatalf("the playing seed was restarted or replaced: plays %v", engine.playCalls[playsBefore:])
	}
	if m.playlist.Len() != 3 {
		t.Fatalf("queue has %d tracks, want the seed and 2 (the station's copy of the seed dropped)", m.playlist.Len())
	}
	if _, idx := m.playlist.Current(); idx != 0 {
		t.Fatalf("current position is %d, want the seed at 0", idx)
	}
	if m.playbackDetached {
		t.Fatal("the seed is still detached, so the next track would replay it")
	}
	next, ok := m.playlist.Next()
	if !ok || next.Path != "spotify:track:a" {
		t.Fatalf("after the seed comes %q, want the station's first track", next.Path)
	}
}

// Radio from a song that is not playing starts with that song.
func TestTrackRadioPlaysTheSeedFirstWhenItWasNotPlaying(t *testing.T) {
	prov := &radioTestProvider{commandsTestProvider: commandsTestProvider{name: "Spotify"}}
	m := radioModel(t, prov)
	seed, _ := m.playlist.Track(0)

	m.applyTrackRadio(trackRadioMsg{seed: seed, tracks: []playlist.Track{{Path: "spotify:track:a"}}, gen: m.requests.tracks})

	if first, _ := m.playlist.Track(0); first.Path != "spotify:track:seed" {
		t.Fatalf("queue starts with %q, want the seed", first.Path)
	}
}
