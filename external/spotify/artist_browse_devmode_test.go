package spotify

import (
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// invalidLimitResponse is Spotify's real reply when a Development Mode app
// asks a catalog endpoint for more than its quota allows — the same 400
// "Invalid limit" search_devmode_test.go already fakes for /v1/search.
// Artists (/v1/me/following) and ArtistAlbums (/v1/artists/{id}/albums) hit
// it too: it's what surfaced as a live "Album load failed: ... Invalid
// limit" error against a real account.
func invalidLimitResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"status":400,"message":"Invalid limit"}}`)),
		Request:    req,
	}
}

func okResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestArtistsFallsBackToDevModeLimit(t *testing.T) {
	var limits []int
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
		limits = append(limits, limit)
		if limit > devModeSearchLimit {
			return invalidLimitResponse(req), nil
		}
		return okResponse(req, `{"artists":{"items":[{"id":"a1","name":"Boards of Canada"}],"cursors":{"after":""}}}`), nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })

	sess := &Session{tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})}
	got, err := New(sess, "client", 320).Artists()
	if err != nil {
		t.Fatalf("Artists() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "a1" {
		t.Errorf("Artists() = %+v, want the single artist from the fallback page", got)
	}
	if want := []int{spotifyArtistPageSize, devModeSearchLimit}; !slices.Equal(limits, want) {
		t.Errorf("requested limits = %v, want %v (full size first, then the fallback)", limits, want)
	}
}

func TestArtistAlbumsFallsBackToDevModeLimitAndKeepsPaging(t *testing.T) {
	const total = 15
	type req struct{ limit, offset int }
	var requests []req

	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		requests = append(requests, req{limit, offset})

		if limit > devModeSearchLimit {
			return invalidLimitResponse(r), nil
		}
		return okResponse(r, albumsPage(offset, limit, total)), nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })

	sess := &Session{tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "token"})}
	got, err := New(sess, "client", 320).ArtistAlbums("a1")
	if err != nil {
		t.Fatalf("ArtistAlbums() error = %v", err)
	}
	if len(got) != total {
		t.Fatalf("got %d albums, want %d", len(got), total)
	}

	// One rejected request at the full size, then two accepted pages of the
	// fallback size (15 albums / 10 per page).
	want := []req{
		{spotifyArtistPageSize, 0},
		{devModeSearchLimit, 0},
		{devModeSearchLimit, devModeSearchLimit},
	}
	if !slices.Equal(requests, want) {
		t.Errorf("requests = %+v, want %+v", requests, want)
	}
}

// albumsPage builds a fake /v1/artists/{id}/albums page of size limit
// starting at offset, out of a catalog of total albums.
func albumsPage(offset, limit, total int) string {
	var items strings.Builder
	items.WriteString(`{"total":`)
	items.WriteString(strconv.Itoa(total))
	items.WriteString(`,"items":[`)
	for i := offset; i < offset+limit && i < total; i++ {
		if i > offset {
			items.WriteString(",")
		}
		items.WriteString(`{"id":"al`)
		items.WriteString(strconv.Itoa(i))
		items.WriteString(`","name":"Album","total_tracks":1,"artists":[{"id":"a1","name":"Artist"}]}`)
	}
	items.WriteString(`]}`)
	return items.String()
}
