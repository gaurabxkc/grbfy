package spotify

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/provider"
)

const (
	// spotifyArtistPageSize is the Web API maximum for both
	// /v1/me/following and /v1/artists/{id}/albums.
	spotifyArtistPageSize = 50

	// artistAlbumGroups leaves out "appears_on" and "compilation" so an
	// artist reads as their own discography rather than every guest credit
	// and every various-artists compilation they ever landed on.
	artistAlbumGroups = "album,single"

	// browseArtistsID is a UI-only pane shortcut into the hierarchical
	// browser; it is never a playable playlist ID.
	browseArtistsID = "browse/artists"

	// yourMusicID mirrors the ID Playlists() gives the Your Music row. It is
	// duplicated rather than shared to keep this feature out of the upstream
	// file it would otherwise have to edit.
	yourMusicID = "YOUR MUSIC"

	// artistBrowseTimeout bounds a full paginated walk, not one request.
	artistBrowseTimeout = 60 * time.Second
)

// followedArtistsPage fetches one page of GET /v1/me/following?type=artist
// and returns the cursor for the next page ("" when the walk is done).
//
// This endpoint is the odd one out in Spotify's API: it pages by cursor
// rather than by the limit/offset every other call in this package uses
// (savedAlbums, Tracks, searchPaged). Items are nested under "artists", and
// the next page is requested with after=<last artist id>.
func (p *SpotifyProvider) followedArtistsPage(ctx context.Context, after string, limit int) ([]spotifyArtist, string, error) {
	query := url.Values{
		"type":  {"artist"},
		"limit": {strconv.Itoa(limit)},
	}
	if after != "" {
		query.Set("after", after)
	}

	resp, err := p.webAPI(ctx, "GET", "/v1/me/following", query)
	if err != nil {
		return nil, "", err
	}

	var result struct {
		Artists struct {
			Items   []spotifyArtist `json:"items"`
			Cursors struct {
				After string `json:"after"`
			} `json:"cursors"`
		} `json:"artists"`
	}
	if err := decodeBody(resp, &result); err != nil {
		return nil, "", fmt.Errorf("spotify: parse followed artists: %w", err)
	}
	return result.Artists.Items, result.Artists.Cursors.After, nil
}

// Artists returns every artist the user follows. Implements
// provider.ArtistBrowser.
//
// The cursor is followed to exhaustion here rather than by the caller: the
// UI calls this once and treats the result as the complete list (see
// fetchNavArtistsCmd), so a partial return would silently truncate.
//
// Development Mode apps cap this endpoint below Spotify's documented 50,
// answering with the same misleading 400 "Invalid limit" that
// /v1/search does (see devModeSearchLimit/isInvalidLimit) — a request for
// the full page size is tried first so an app with Extended Quota Mode
// keeps its single request, falling back to smaller pages only if rejected.
func (p *SpotifyProvider) Artists() ([]provider.ArtistInfo, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), artistBrowseTimeout)
	defer cancel()

	limit := spotifyArtistPageSize
	var out []provider.ArtistInfo
	after := ""
	for {
		items, next, err := p.followedArtistsPage(ctx, after, limit)
		if err != nil && after == "" && isInvalidLimit(err) && limit > devModeSearchLimit {
			limit = devModeSearchLimit
			items, next, err = p.followedArtistsPage(ctx, after, limit)
		}
		if err != nil {
			return nil, fmt.Errorf("spotify: followed artists: %w", err)
		}
		for _, a := range items {
			if a.ID == "" {
				continue
			}
			// AlbumCount stays 0: /v1/me/following doesn't carry it and
			// filling it would cost one request per artist. Tidal's
			// ArtistBrowser leaves it unset for the same reason.
			out = append(out, provider.ArtistInfo{ID: a.ID, Name: a.Name})
		}
		if next == "" || len(items) == 0 {
			return out, nil
		}
		after = next
	}
}

// artistAlbumsPage fetches one limit/offset page of an artist's albums and
// reports the total the API claims, so the caller knows when to stop.
func (p *SpotifyProvider) artistAlbumsPage(ctx context.Context, artistID string, offset, limit int) ([]spotifyAlbumItem, int, error) {
	query := url.Values{
		"include_groups": {artistAlbumGroups},
		"limit":          {strconv.Itoa(limit)},
		"offset":         {strconv.Itoa(offset)},
	}

	resp, err := p.webAPI(ctx, "GET", "/v1/artists/"+url.PathEscape(artistID)+"/albums", query)
	if err != nil {
		return nil, 0, err
	}

	var result struct {
		Items []spotifyAlbumItem `json:"items"`
		Total int                `json:"total"`
	}
	if err := decodeBody(resp, &result); err != nil {
		return nil, 0, fmt.Errorf("spotify: parse artist albums: %w", err)
	}
	return result.Items, result.Total, nil
}

// ArtistAlbums returns an artist's albums and singles, newest-first as the
// API orders them. Implements provider.ArtistBrowser.
//
// Like Artists, this paginates to completion: the UI calls it once and marks
// the result as the last page. It hits the same Development Mode "Invalid
// limit" quota /v1/search does — see Artists's comment for the mechanism —
// so the full page size is tried first and only shrunk on rejection.
func (p *SpotifyProvider) ArtistAlbums(artistID string) ([]provider.AlbumInfo, error) {
	if err := p.ensureSession(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), artistBrowseTimeout)
	defer cancel()

	limit := spotifyArtistPageSize
	var out []provider.AlbumInfo
	seen := map[string]bool{}
	fetched := 0

	for offset := 0; ; offset += limit {
		items, total, err := p.artistAlbumsPage(ctx, artistID, offset, limit)
		if err != nil && offset == 0 && isInvalidLimit(err) && limit > devModeSearchLimit {
			limit = devModeSearchLimit
			items, total, err = p.artistAlbumsPage(ctx, artistID, offset, limit)
		}
		if err != nil {
			return nil, fmt.Errorf("spotify: artist %s albums: %w", artistID, err)
		}

		for _, a := range items {
			// The same album comes back more than once when it's released
			// across several markets. Dedupe on ID only: same-named
			// reissues are genuinely different releases and collapsing
			// them by name would hide them.
			if a.ID == "" || seen[a.ID] {
				continue
			}
			seen[a.ID] = true

			info := provider.AlbumInfo{
				ID:         a.ID,
				Name:       a.Name,
				Artist:     artistNames(a.Artists),
				ArtistID:   artistID,
				Year:       provider.YearFromDate(a.ReleaseDate),
				TrackCount: a.TotalTracks,
			}
			if len(a.Artists) > 0 && a.Artists[0].ID != "" {
				info.ArtistID = a.Artists[0].ID
			}
			out = append(out, info)
		}

		fetched += len(items)
		if fetched >= total || len(items) < limit {
			return p.withTopTracks(artistID, out), nil
		}
	}
}

// ArtistForTrack resolves a Spotify track back to its primary artist so the
// browser can jump straight to that artist's albums. Implements
// provider.TrackArtistResolver.
//
// The ID was stashed in ProviderMeta when the track was parsed, so this does
// no I/O. Podcast episodes carry no artist and resolve to false.
func (p *SpotifyProvider) ArtistForTrack(track playlist.Track) (provider.ArtistInfo, bool) {
	id := track.Meta(provider.MetaSpotifyArtistID)
	if id == "" {
		return provider.ArtistInfo{}, false
	}

	// Track.Artist joins every credited artist with ", "; the stashed ID is
	// the first one, so take the first name to match it.
	name := track.Artist
	if before, _, found := strings.Cut(name, ", "); found {
		name = before
	}
	return provider.ArtistInfo{ID: id, Name: name}, true
}

// BrowseEntries adds a Followed Artists shortcut to the provider pane's
// Library section, right after Your Music. Implements
// provider.BrowseEntryProvider.
//
// Anchoring on the Your Music row rather than on the Saved albums section is
// deliberate: Your Music is added unconditionally by Playlists(), so the
// anchor is always present, while a user with no saved albums has no
// Saved albums section for the entry to attach to.
func (p *SpotifyProvider) BrowseEntries() []provider.BrowseEntry {
	return []provider.BrowseEntry{
		{
			ID: browseArtistsID, Name: "Followed Artists", Section: "Library",
			Mode: provider.BrowseArtistAlbums, AfterID: yourMusicID,
			AfterSection: "Library",
		},
	}
}

// withTopTracks puts the artist's top tracks ahead of their albums, where the
// Spotify app puts them too.
func (p *SpotifyProvider) withTopTracks(artistID string, albums []provider.AlbumInfo) []provider.AlbumInfo {
	name := ""
	if len(albums) > 0 {
		name = albums[0].Artist
	}
	entry, ok := p.topTracksEntry(artistID, name)
	if !ok {
		return albums
	}
	return append([]provider.AlbumInfo{entry}, albums...)
}
