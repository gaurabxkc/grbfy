package spotify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
	librespot "github.com/devgianlu/go-librespot"
	extmetapb "github.com/devgianlu/go-librespot/proto/spotify/extendedmetadata"
	metadatapb "github.com/devgianlu/go-librespot/proto/spotify/metadata"
)

// topTracksPrefix marks the album ID of an artist's top-tracks entry. It is
// not a Spotify ID, so AlbumTracks recognizes it before asking the Web API.
const topTracksPrefix = "top:"

// topTracksLookup bounds the extra request ArtistAlbums makes. The album list
// is what the user asked for; top tracks are a bonus, and a slow answer should
// cost the entry, not the list.
const topTracksLookup = 5 * time.Second

// artistTopTrackURIs asks the client protocol for an artist's most-played
// tracks. The Web API has an endpoint for this too, but a Development Mode
// registration is refused most of the artist endpoints, and the protocol
// answers with the same list Spotify's own app shows.
//
// Spotify sends the list for the account's region, so the first non-empty
// one is taken.
func (p *SpotifyProvider) artistTopTrackURIs(ctx context.Context, artistID string) ([]string, error) {
	sess := p.session
	if sess == nil || sess.sess == nil {
		return nil, fmt.Errorf("no session")
	}

	uri := "spotify:artist:" + artistID
	sess.mu.RLock()
	client := sess.sess.Spclient()
	sess.mu.RUnlock()

	res, err := client.ExtendedMetadata(ctx, &extmetapb.BatchedEntityRequest{EntityRequest: []*extmetapb.EntityRequest{{
		EntityUri: uri,
		Query:     []*extmetapb.ExtensionQuery{{ExtensionKind: extmetapb.ExtensionKind_ARTIST_V4}},
	}}})
	if err != nil {
		return nil, err
	}

	var artist metadatapb.Artist
	for _, ext := range res.GetExtendedMetadata() {
		for _, d := range ext.GetExtensionData() {
			if err := d.GetExtensionData().UnmarshalTo(&artist); err == nil {
				break
			}
		}
	}

	for _, list := range artist.GetTopTrack() {
		var uris []string
		for _, tr := range list.GetTrack() {
			if gid := tr.GetGid(); len(gid) > 0 {
				uris = append(uris, librespot.SpotifyIdFromGid(librespot.SpotifyIdTypeTrack, gid).Uri())
			}
		}
		if len(uris) > 0 {
			return uris, nil
		}
	}
	return nil, nil
}

// topTracksEntry is the entry ArtistAlbums shows first: an artist's top
// tracks, opened like any album. It is left out when the artist has none or
// the lookup fails, so the list never offers an entry that cannot open.
func (p *SpotifyProvider) topTracksEntry(artistID, artistName string) (provider.AlbumInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), topTracksLookup)
	defer cancel()
	uris, err := p.artistTopTrackURIs(ctx, artistID)
	if err != nil || len(uris) == 0 {
		return provider.AlbumInfo{}, false
	}
	return provider.AlbumInfo{
		ID:         topTracksPrefix + artistID,
		Name:       "Top tracks",
		Artist:     artistName,
		ArtistID:   artistID,
		TrackCount: len(uris),
		Synthetic:  true,
	}, true
}

// artistTopTracks opens a top-tracks entry.
func (p *SpotifyProvider) artistTopTracks(ctx context.Context, albumID string) ([]playlist.Track, error) {
	artistID := strings.TrimPrefix(albumID, topTracksPrefix)
	uris, err := p.artistTopTrackURIs(ctx, artistID)
	if err != nil {
		return nil, fmt.Errorf("spotify: top tracks for %s: %w", artistID, err)
	}
	if len(uris) == 0 {
		return nil, fmt.Errorf("spotify: %s has no top tracks", artistID)
	}
	return p.trackMetadata(ctx, uris)
}
