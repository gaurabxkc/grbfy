package spotify

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"

	"github.com/bjarneo/cliamp/applog"
	"github.com/bjarneo/cliamp/playlist"
)

// spotifyFolderIDPrefix marks a PlaylistInfo.ID as a synthetic "play this
// whole folder" entry rather than a real playlist, so Tracks() routes it to
// folderTracks. Real playlist/album IDs never contain a colon-delimited
// "folder" segment, so this can't collide with one.
const spotifyFolderIDPrefix = "spotify:folder:"

// isSpotifyFolderID reports whether id names a synthetic folder entry and, if
// so, returns the folder path it stands for.
func isSpotifyFolderID(id string) (path string, ok bool) {
	path, ok = strings.CutPrefix(id, spotifyFolderIDPrefix)
	return path, ok
}

// rootlistInfo is the folder hierarchy extracted from a user's rootlist.
type rootlistInfo struct {
	// folderOf maps a playlist ID to the immediate folder path it's placed
	// in (nested folders joined with " / "). Absent means "not in a folder".
	folderOf map[string]string
	// order maps a playlist ID to its position within the rootlist, so
	// callers can preserve the user's own ordering within a folder.
	order map[string]int
	// paths lists every distinct folder path encountered, at every nesting
	// depth, in first-seen rootlist order (e.g. both "Study" and
	// "Study / Focus" appear as separate entries).
	paths []string
	// children maps a folder path to every playlist ID recursively
	// contained within it — including anything nested in subfolders — in
	// rootlist order. Playing a folder plays everything in this slice.
	children map[string][]string
}

// rootlistFolders fetches the authenticated user's Spotify playlist-folder
// hierarchy.
//
// The rootlist has no public Web API endpoint (see Session.rootlistItems),
// so any failure here — a stale spclient token, a protocol change, no live
// AP connection yet — is logged and swallowed rather than propagated: the
// caller falls back to the flat Section grouping instead of breaking
// playlist loading entirely.
func (p *SpotifyProvider) rootlistFolders(ctx context.Context) rootlistInfo {
	empty := rootlistInfo{
		folderOf: map[string]string{},
		order:    map[string]int{},
		children: map[string][]string{},
	}

	p.mu.Lock()
	sess := p.session
	p.mu.Unlock()
	if sess == nil {
		return empty
	}

	items, err := sess.rootlistItems(ctx)
	if err != nil {
		applog.Warn("spotify: rootlist unavailable, showing flat playlist list: %v", err)
		return empty
	}

	return parseRootlistItems(items)
}

// parseRootlistItems walks a flat rootlist item sequence, tracking folder
// nesting via "spotify:start-group:<gid>:<url-encoded name>" /
// "spotify:end-group:<gid>" marker entries that bracket the playlists they
// contain. A playlist is recorded as a child of every folder currently open
// on the stack, not just the innermost one, so playing an outer folder plays
// its subfolders' tracks too. Unrecognized item kinds (future rootlist entry
// types) are skipped rather than treated as errors.
func parseRootlistItems(items []*playlist4pb.Item) rootlistInfo {
	info := rootlistInfo{
		folderOf: map[string]string{},
		order:    map[string]int{},
		children: map[string][]string{},
	}
	seenPath := map[string]bool{}

	var stack []string
	pos := 0
	for _, item := range items {
		uri := item.GetUri()
		switch {
		case strings.HasPrefix(uri, "spotify:start-group:"):
			rest := strings.TrimPrefix(uri, "spotify:start-group:")
			_, name, ok := strings.Cut(rest, ":")
			if !ok {
				name = rest
			}
			if decoded, err := url.QueryUnescape(name); err == nil {
				name = decoded
			}
			stack = append(stack, name)
			path := strings.Join(stack, " / ")
			if !seenPath[path] {
				seenPath[path] = true
				info.paths = append(info.paths, path)
			}
		case strings.HasPrefix(uri, "spotify:end-group:"):
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case strings.HasPrefix(uri, "spotify:playlist:"):
			id := strings.TrimPrefix(uri, "spotify:playlist:")
			info.order[id] = pos
			pos++
			if len(stack) > 0 {
				info.folderOf[id] = strings.Join(stack, " / ")
				for i := 1; i <= len(stack); i++ {
					ancestor := strings.Join(stack[:i], " / ")
					info.children[ancestor] = append(info.children[ancestor], id)
				}
			}
		}
	}
	return info
}

// folderTracks returns the concatenated tracks of every playlist recursively
// contained in the given folder path, in rootlist order, so playing a
// folder mixes all of its playlists (and subfolders) into one queue. A
// child playlist that fails to load (region-locked, deleted, transient API
// error) is skipped with a warning rather than failing the whole folder;
// the folder only errors if every child failed.
func (p *SpotifyProvider) folderTracks(path string) ([]playlist.Track, error) {
	p.mu.Lock()
	ids := slices.Clone(p.folderChildren[path])
	p.mu.Unlock()
	if len(ids) == 0 {
		return nil, fmt.Errorf("spotify: folder %q has no playlists (try refreshing)", path)
	}

	var all []playlist.Track
	var lastErr error
	for _, id := range ids {
		tracks, err := p.Tracks(id)
		if err != nil {
			applog.Warn("spotify: folder %q: skipping playlist %s: %v", path, id, err)
			lastErr = err
			continue
		}
		all = append(all, tracks...)
	}
	if len(all) == 0 && lastErr != nil {
		return nil, fmt.Errorf("spotify: folder %q: every playlist failed to load: %w", path, lastErr)
	}
	return all, nil
}

// folderLeafName returns the last segment of a " / "-joined folder path, for
// a short display name on the folder's synthetic "play all" row.
func folderLeafName(path string) string {
	parts := strings.Split(path, " / ")
	return parts[len(parts)-1]
}
