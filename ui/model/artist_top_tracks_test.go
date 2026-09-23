package model

import (
	"testing"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// topTracksTestProvider has one real album and one provider-assembled list
// that repeats a track from it.
type topTracksTestProvider struct {
	commandsTestProvider
}

func (topTracksTestProvider) Artists() ([]provider.ArtistInfo, error) { return nil, nil }

func (topTracksTestProvider) ArtistAlbums(string) ([]provider.AlbumInfo, error) {
	return []provider.AlbumInfo{
		{ID: "top:artist", Name: "Top tracks", Synthetic: true},
		{ID: "album1", Name: "Album"},
	}, nil
}

func (topTracksTestProvider) AlbumTracks(id string) ([]playlist.Track, error) {
	if id == "top:artist" {
		return []playlist.Track{{Path: "spotify:track:hit"}}, nil
	}
	return []playlist.Track{{Path: "spotify:track:hit"}, {Path: "spotify:track:deep-cut"}}, nil
}

// Gathering every track by an artist must not count the top-tracks list: its
// songs are already on the albums, so they would play twice.
func TestArtistAllTracksSkipsTheTopTracksList(t *testing.T) {
	prov := topTracksTestProvider{commandsTestProvider{name: "Spotify"}}
	m := Model{}
	m.navBrowser.prov = prov

	msg := m.fetchNavArtistAllTracksCmd(prov, "artist")().(navTracksLoadedMsg)
	if msg.err != nil {
		t.Fatalf("loading the artist's tracks: %v", msg.err)
	}
	if len(msg.tracks) != 2 {
		t.Fatalf("got %d tracks, want the album's 2 without the top-tracks repeat", len(msg.tracks))
	}
}
