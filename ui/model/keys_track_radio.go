package model

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// radioInterval is the least time between two stations starting. Starting one
// asks for the station, then opens its first track and preloads the second
// straight away, and Spotify refuses audio keys when track opens come too fast
// for too long. Moving through a station already playing is one open per track
// and is not limited. Held in memory, so it resets with grbfy.
const radioInterval = 10 * time.Second

// trackRadioState keeps one station request from overlapping another, and
// spaces out how often a new one can start.
type trackRadioState struct {
	starting  bool
	lastStart time.Time
}

// trackRadioMsg carries a station built from a track back to the model.
type trackRadioMsg struct {
	seed   playlist.Track
	tracks []playlist.Track
	gen    uint64
	err    error
}

// startTrackRadio replaces the queue with the station the provider builds from
// the selected track. Providers that cannot do it say so rather than failing
// quietly.
func (m *Model) startTrackRadio() tea.Cmd {
	starter, ok := m.provider.(provider.RadioStarter)
	if !ok {
		name := "This provider"
		if m.provider != nil {
			name = m.provider.Name()
		}
		m.status.Warningf(statusTTLDefault, "%s cannot start a radio", name)
		return nil
	}
	if m.focus != focusPlaylist || m.playlist == nil || m.plCursor < 0 || m.plCursor >= m.playlist.Len() {
		m.status.Warning("Select a track to start its radio", statusTTLDefault)
		return nil
	}
	// A held key repeats, and nothing upstream filters that, so without this
	// every repeat would be a request of its own.
	if m.trackRadio.starting {
		m.status.Activityf(statusTTLDefault, "Radio is still starting…")
		return nil
	}
	if wait := radioInterval - time.Since(m.trackRadio.lastStart); wait > 0 {
		m.status.Showf(statusTTLDefault, "Next radio available in %ds", int(wait.Round(time.Second).Seconds()))
		return nil
	}
	track, ok := m.playlist.Track(m.plCursor)
	if !ok {
		return nil
	}

	m.trackRadio.starting = true
	m.status.Activityf(statusTTLDefault, "Starting radio from %s…", trackViewName(track))
	return startTrackRadioCmd(starter, track, nextRequest(&m.requests.tracks))
}

// startTrackRadioCmd asks the provider for the station a track seeds. The
// deadline is its own: a station is one resolve plus its metadata, so it
// should answer quickly or not at all.
func startTrackRadioCmd(starter provider.RadioStarter, seed playlist.Track, gen uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tracks, err := starter.TrackRadio(ctx, seed.Path)
		return trackRadioMsg{seed: seed, tracks: tracks, gen: gen, err: err}
	}
}

// applyTrackRadio puts a finished station on screen and starts it.
func (m *Model) applyTrackRadio(msg trackRadioMsg) tea.Cmd {
	m.trackRadio.starting = false
	if msg.gen != m.requests.tracks {
		return nil // a newer request replaced this one
	}
	if msg.err != nil {
		m.status.Errorf(statusTTLDefault, "Radio failed: %s", msg.err)
		return nil
	}
	if len(msg.tracks) == 0 {
		m.status.Warning("That track has no radio", statusTTLDefault)
		return nil
	}

	// The seed leads the station, as in Spotify's own "Go to song radio":
	// the song you asked about is part of its radio, not replaced by it. A
	// station can name the seed itself too, so it is dropped from there.
	tracks := make([]playlist.Track, 0, len(msg.tracks)+1)
	tracks = append(tracks, msg.seed)
	for _, t := range msg.tracks {
		if t.Path != msg.seed.Path {
			tracks = append(tracks, t)
		}
	}

	// Asked about the song already playing: let it play on. It becomes the
	// first entry and the station follows it, instead of cutting it off to
	// start the station's first track.
	playingSeed := false
	if current, idx := m.currentPlaybackTrack(); idx >= 0 && current.Path == msg.seed.Path &&
		(m.buffering || m.player.IsPlaying()) {
		playingSeed = true
	}

	m.trackRadio.lastStart = time.Now()
	m.retireTracksPaging()
	m.replacePlayerPlaylist(tracks)
	m.activeProviderPlaylistID = ""
	m.plCursor = 0
	m.adjustScroll()
	m.status.Successf(statusTTLDefault, "Radio from %s: %d tracks", trackViewName(msg.seed), len(tracks)-1)
	m.notifyAll()

	if playingSeed {
		// replacePlayerPlaylist detached the playing song. Left detached, it
		// would be followed by whatever sits at the current position, which is
		// the seed again. Re-attached at position 0, "next" is the station.
		m.playlist.SetIndex(0)
		if seed, idx := m.playlist.Current(); idx == 0 {
			m.setPlaybackTrack(seed)
		}
		return m.rearmPreload()
	}
	return m.playCurrentTrack()
}
