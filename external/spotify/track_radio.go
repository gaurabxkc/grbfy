package spotify

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/bjarneo/cliamp/playlist"
	extmetapb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
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

// trackRadioBatch matches what Spotify's own clients send in one extended
// metadata request, so this traffic is shaped like every other client
// speaking the protocol.
const trackRadioBatch = 100

// spotifyImageHost is where a cover file id resolves to an actual image.
const spotifyImageHost = "https://i.scdn.co/image/"

// metadataImageWidth fills in the width for images that carry a size class
// rather than pixel dimensions.
var metadataImageWidth = map[metadatapb.Image_Size]int{
	metadatapb.Image_DEFAULT: 300,
	metadatapb.Image_SMALL:   64,
	metadatapb.Image_LARGE:   640,
	metadatapb.Image_XLARGE:  640,
}

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

// stationMetadata fills in one batch of station URIs.
//
// This goes through the client protocol rather than the Web API. /v1/tracks
// answers 403 to a Development Mode registration, which is every personal
// client_id, and the station is useless without titles.
func (p *SpotifyProvider) stationMetadata(ctx context.Context, uris []string) ([]playlist.Track, error) {
	if len(uris) == 0 {
		return nil, nil
	}
	sess := p.session
	if sess == nil || sess.sess == nil {
		return nil, fmt.Errorf("spotify: radio metadata: no session")
	}

	reqs := make([]*extmetapb.EntityRequest, 0, len(uris))
	for _, uri := range uris {
		reqs = append(reqs, &extmetapb.EntityRequest{
			EntityUri: uri,
			Query:     []*extmetapb.ExtensionQuery{{ExtensionKind: extmetapb.ExtensionKind_TRACK_V4}},
		})
	}

	sess.mu.RLock()
	client := sess.sess.Spclient()
	sess.mu.RUnlock()

	res, err := client.ExtendedMetadata(ctx, &extmetapb.BatchedEntityRequest{EntityRequest: reqs})
	if err != nil {
		return nil, fmt.Errorf("spotify: radio metadata: %w", err)
	}

	// The response is not ordered like the request, so index it and rebuild
	// the station in the order Spotify chose for it.
	byURI := make(map[string]playlist.Track, len(uris))
	for _, ext := range res.GetExtendedMetadata() {
		for _, d := range ext.GetExtensionData() {
			var tr metadatapb.Track
			if err := d.GetExtensionData().UnmarshalTo(&tr); err != nil {
				continue // a track Spotify declines to describe is skipped
			}
			byURI[d.GetEntityUri()] = trackFromMetadata(d.GetEntityUri(), &tr)
		}
	}

	out := make([]playlist.Track, 0, len(uris))
	for _, uri := range uris {
		if t, ok := byURI[uri]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// trackFromMetadata converts Spotify's internal track message into a track.
func trackFromMetadata(uri string, tr *metadatapb.Track) playlist.Track {
	names := make([]string, 0, len(tr.GetArtist()))
	for _, a := range tr.GetArtist() {
		if n := a.GetName(); n != "" {
			names = append(names, n)
		}
	}
	return playlist.Track{
		Path:         uri,
		Title:        tr.GetName(),
		Artist:       strings.Join(names, ", "),
		Album:        tr.GetAlbum().GetName(),
		Year:         int(tr.GetAlbum().GetDate().GetYear()),
		AlbumArtURL:  coverFromMetadata(tr.GetAlbum()),
		DurationSecs: int(tr.GetDuration()) / 1000,
		TrackNumber:  int(tr.GetNumber()),
	}
}

// coverFromMetadata picks the album art the same way the Web API path does,
// from the cover group the protocol carries instead of a list of URLs.
func coverFromMetadata(album *metadatapb.Album) string {
	sources := album.GetCoverGroup().GetImage()
	if len(sources) == 0 {
		sources = album.GetCover()
	}
	images := make([]spotifyImage, 0, len(sources))
	for _, img := range sources {
		if len(img.GetFileId()) == 0 {
			continue
		}
		width := int(img.GetWidth())
		if width == 0 {
			width = metadataImageWidth[img.GetSize()]
		}
		images = append(images, spotifyImage{
			URL:    spotifyImageHost + hex.EncodeToString(img.GetFileId()),
			Width:  width,
			Height: int(img.GetHeight()),
		})
	}
	return pickCoverImage(images)
}
