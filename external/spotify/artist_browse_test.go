package spotify

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

// fakeSpotifyAPI installs a transport answering the given path->body map and
// records every request URL, mirroring the harness in
// provider_playlists_test.go.
func fakeSpotifyAPI(t *testing.T, handler func(*http.Request) (string, error)) *[]string {
	t.Helper()
	var seen []string
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.URL.String())
		body, err := handler(req)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	return &seen
}

func testProvider() *SpotifyProvider {
	sess := &Session{tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})}
	return New(sess, "client", 320)
}

func TestArtistsFollowsCursorPagination(t *testing.T) {
	seen := fakeSpotifyAPI(t, func(req *http.Request) (string, error) {
		if req.URL.Path != "/v1/me/following" {
			return "", fmt.Errorf("unexpected path %q", req.URL.Path)
		}
		switch req.URL.Query().Get("after") {
		case "":
			return `{"artists":{"items":[` +
				`{"id":"a1","name":"Boards of Canada"},` +
				`{"id":"a2","name":"Aphex Twin"}` +
				`],"cursors":{"after":"a2"}}}`, nil
		case "a2":
			return `{"artists":{"items":[` +
				`{"id":"a3","name":"Autechre"}` +
				`],"cursors":{"after":""}}}`, nil
		default:
			return "", fmt.Errorf("unexpected after cursor %q", req.URL.Query().Get("after"))
		}
	})

	got, err := testProvider().Artists()
	if err != nil {
		t.Fatal(err)
	}

	want := []provider.ArtistInfo{
		{ID: "a1", Name: "Boards of Canada"},
		{ID: "a2", Name: "Aphex Twin"},
		{ID: "a3", Name: "Autechre"},
	}
	if len(got) != len(want) {
		t.Fatalf("Artists() returned %d artists, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Artists()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(*seen) != 2 {
		t.Fatalf("expected 2 requests (one per page), got %d: %v", len(*seen), *seen)
	}
	if !strings.Contains((*seen)[1], "after=a2") {
		t.Errorf("second request should carry the first page's cursor, got %q", (*seen)[1])
	}
}

func TestArtistsSkipsEntriesWithoutID(t *testing.T) {
	fakeSpotifyAPI(t, func(*http.Request) (string, error) {
		return `{"artists":{"items":[` +
			`{"id":"","name":"Ghost"},` +
			`{"id":"a1","name":"Real"}` +
			`],"cursors":{"after":""}}}`, nil
	})

	got, err := testProvider().Artists()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a1" {
		t.Errorf("Artists() = %+v, want only the entry with an ID", got)
	}
}

func TestArtistAlbumsPaginatesAndDedupes(t *testing.T) {
	// Page size is 50, so page one must be full for the loop to continue.
	var page1 strings.Builder
	page1.WriteString(`{"total":51,"items":[`)
	for i := range spotifyArtistPageSize {
		if i > 0 {
			page1.WriteString(",")
		}
		fmt.Fprintf(&page1, `{"id":"al%d","name":"Album %d","total_tracks":10,"release_date":"2019-05-01","artists":[{"id":"a1","name":"Artist"}]}`, i, i)
	}
	page1.WriteString(`]}`)

	seen := fakeSpotifyAPI(t, func(req *http.Request) (string, error) {
		if req.URL.Path != "/v1/artists/a1/albums" {
			return "", fmt.Errorf("unexpected path %q", req.URL.Path)
		}
		if req.URL.Query().Get("offset") == "0" {
			return page1.String(), nil
		}
		// Page two repeats al0 (same album across markets) plus one new one.
		return `{"total":51,"items":[` +
			`{"id":"al0","name":"Album 0","total_tracks":10,"release_date":"2019-05-01","artists":[{"id":"a1","name":"Artist"}]},` +
			`{"id":"al99","name":"Last One","total_tracks":3,"release_date":"2021","artists":[{"id":"a1","name":"Artist"}]}` +
			`]}`, nil
	})

	got, err := testProvider().ArtistAlbums("a1")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != spotifyArtistPageSize+1 {
		t.Fatalf("ArtistAlbums() returned %d albums, want %d (duplicate dropped)", len(got), spotifyArtistPageSize+1)
	}
	if got[len(got)-1].ID != "al99" {
		t.Errorf("last album = %q, want al99", got[len(got)-1].ID)
	}
	if len(*seen) != 2 {
		t.Fatalf("expected 2 page requests, got %d: %v", len(*seen), *seen)
	}
	if !strings.Contains((*seen)[0], "include_groups=album%2Csingle") {
		t.Errorf("request should limit include_groups, got %q", (*seen)[0])
	}
}

func TestArtistAlbumsMapsFields(t *testing.T) {
	fakeSpotifyAPI(t, func(*http.Request) (string, error) {
		return `{"total":2,"items":[` +
			`{"id":"al1","name":"Geogaddi","total_tracks":23,"release_date":"2002-02-18","artists":[{"id":"a1","name":"Boards of Canada"}]},` +
			`{"id":"al2","name":"No Artist","total_tracks":1,"release_date":"1999"}` +
			`]}`, nil
	})

	got, err := testProvider().ArtistAlbums("queried")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d albums, want 2", len(got))
	}

	want := provider.AlbumInfo{
		ID: "al1", Name: "Geogaddi", Artist: "Boards of Canada",
		ArtistID: "a1", Year: 2002, TrackCount: 23,
	}
	if got[0] != want {
		t.Errorf("album[0] = %+v, want %+v", got[0], want)
	}
	// With no artist on the album, ArtistID falls back to the queried artist.
	if got[1].ArtistID != "queried" {
		t.Errorf("album[1].ArtistID = %q, want the queried artist ID", got[1].ArtistID)
	}
	if got[1].Year != 1999 {
		t.Errorf("album[1].Year = %d, want 1999", got[1].Year)
	}
}

func TestArtistForTrack(t *testing.T) {
	p := testProvider()

	track := playlist.Track{
		Artist:       "Daft Punk, Pharrell Williams",
		ProviderMeta: map[string]string{provider.MetaSpotifyArtistID: "art1"},
	}
	got, ok := p.ArtistForTrack(track)
	if !ok {
		t.Fatal("ArtistForTrack() returned false for a track carrying an artist ID")
	}
	want := provider.ArtistInfo{ID: "art1", Name: "Daft Punk"}
	if got != want {
		t.Errorf("ArtistForTrack() = %+v, want %+v", got, want)
	}

	// A podcast episode (or any track parsed before this key existed).
	if _, ok := p.ArtistForTrack(playlist.Track{Artist: "Some Show"}); ok {
		t.Error("ArtistForTrack() should return false without the artist-ID meta key")
	}
}

func TestTrackFromItemStashesPrimaryArtistID(t *testing.T) {
	got := trackFromItem(&spotifyItem{
		ID: "t1", Name: "One More Time", Type: "track", URI: "spotify:track:t1",
		Artists: []spotifyArtist{{ID: "art1", Name: "Daft Punk"}, {ID: "art2", Name: "Other"}},
	})
	if id := got.Meta(provider.MetaSpotifyArtistID); id != "art1" {
		t.Errorf("track meta artist ID = %q, want the first artist's ID art1", id)
	}

	episode := trackFromItem(&spotifyItem{ID: "e1", Name: "Ep", Type: "episode", URI: "spotify:episode:e1"})
	if id := episode.Meta(provider.MetaSpotifyArtistID); id != "" {
		t.Errorf("episode should carry no artist ID, got %q", id)
	}
}

func TestBrowseEntriesAnchorsAfterYourMusic(t *testing.T) {
	entries := testProvider().BrowseEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d browse entries, want 1", len(entries))
	}
	got := entries[0]
	if got.Mode != provider.BrowseArtistAlbums {
		t.Errorf("Mode = %v, want BrowseArtistAlbums", got.Mode)
	}
	if got.Section != "Library" || got.AfterSection != "Library" {
		t.Errorf("entry should sit in the Library section, got Section=%q AfterSection=%q", got.Section, got.AfterSection)
	}
	if got.AfterID != yourMusicID {
		t.Errorf("AfterID = %q, want the Your Music row %q", got.AfterID, yourMusicID)
	}
}
