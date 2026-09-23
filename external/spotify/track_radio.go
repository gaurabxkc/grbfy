package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bjarneo/cliamp/playlist"
	playerpb "github.com/devgianlu/go-librespot/proto/spotify/player"
)

// stationURIs asks Spotify for the station it would build from a track: what
// its own clients call song radio. The Web API cannot do this at all. Its
// /v1/recommendations endpoint was withdrawn for apps registered after
// 2024-11-27, and there is no replacement, so this goes through the same
// client protocol the player already speaks.
//
// The station comes back as an ordinary context, a list of pages of track
// URIs, which is then filled in with metadata like any other list.
func (s *Session) stationURIs(ctx context.Context, trackURI string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.sess == nil {
		return nil, fmt.Errorf("session closed")
	}

	uri := trackURI
	station, err := s.sess.Spclient().ContextResolveAutoplay(ctx, &playerpb.AutoplayContextRequest{ContextUri: &uri})
	if err != nil {
		return nil, fmt.Errorf("station request: %w", err)
	}

	var uris []string
	for _, page := range station.GetPages() {
		for _, track := range page.GetTracks() {
			if u := track.GetUri(); strings.HasPrefix(u, "spotify:track:") {
				uris = append(uris, u)
			}
		}
	}
	return uris, nil
}

// trackRadioBatch is the most ids /v1/tracks accepts in one request.
const trackRadioBatch = 50

// TrackRadio returns the station Spotify builds from the given track, ready to
// play. The seed itself is left out: it is the track just heard, and a station
// that opens by repeating it reads as a bug.
//
// Implements provider.RadioStarter.
func (p *SpotifyProvider) TrackRadio(ctx context.Context, trackPath string) ([]playlist.Track, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(trackPath, "spotify:track:") {
		return nil, fmt.Errorf("spotify: radio: %q is not a Spotify track", trackPath)
	}
	if p.session == nil {
		return nil, fmt.Errorf("spotify: radio: no session")
	}

	uris, err := p.session.stationURIs(ctx, trackPath)
	if err != nil {
		return nil, fmt.Errorf("spotify: radio for %q: %w", trackPath, err)
	}
	if len(uris) == 0 {
		return nil, fmt.Errorf("spotify: radio for %q: Spotify returned an empty station", trackPath)
	}

	tracks := make([]playlist.Track, 0, len(uris))
	for start := 0; start < len(uris); start += trackRadioBatch {
		end := min(start+trackRadioBatch, len(uris))
		batch, err := p.stationMetadata(ctx, uris[start:end])
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, batch...)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("spotify: radio for %q: no playable tracks in the station", trackPath)
	}
	return tracks, nil
}

// stationMetadata fills in one batch of station URIs through /v1/tracks, which
// a Development Mode registration is still allowed to call.
func (p *SpotifyProvider) stationMetadata(ctx context.Context, uris []string) ([]playlist.Track, error) {
	ids := make([]string, 0, len(uris))
	for _, u := range uris {
		ids = append(ids, strings.TrimPrefix(u, "spotify:track:"))
	}

	resp, err := p.webAPI(ctx, "GET", "/v1/tracks", url.Values{"ids": {strings.Join(ids, ",")}})
	if err != nil {
		return nil, fmt.Errorf("spotify: radio metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		Tracks []*spotifyItem `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("spotify: radio metadata: %w", err)
	}

	tracks := make([]playlist.Track, 0, len(payload.Tracks))
	for _, item := range payload.Tracks {
		// A station can name a track this account cannot play, which comes
		// back as null rather than an error.
		if item == nil || item.ID == "" {
			continue
		}
		tracks = append(tracks, trackFromItem(item))
	}
	return tracks, nil
}
