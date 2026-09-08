# CLAUDE.md — grbfy

> A personal soft fork of [cliamp](https://github.com/bjarneo/cliamp) (Go + Bubbletea terminal
> music player). This file explains what the fork changes and the rules that keep it mergeable.

## What this fork is

grbfy is cliamp with a small set of personal features. It is **not** a hard fork: it deliberately
keeps upstream's Go module path so that upstream changes keep merging cleanly.

Upstream is large (~69.5k lines of non-test Go, 263 files) and very active (63 minor releases in v1
before v2.0.1, commits landing most days). Staying mergeable is the top architectural constraint.

Fork base: `v2.0.1`. Upstream is tracked as the `upstream` remote.

```sh
git fetch upstream
git rebase upstream/main     # feature commits are meant to rebase, not merge
```

## Fork rules — read before changing anything

1. **Never rename the Go module path.** It stays `github.com/bjarneo/cliamp`. It is invisible at
   runtime, and renaming it would rewrite 480 import references across 153 files, conflicting with
   essentially every future upstream merge. This is the single most important rule here.
2. **New features go in new files.** New files never conflict on merge. When a new feature must
   touch an upstream file, touch the minimum: one table row, one key case, one setter call.
3. **Prefer a Lua plugin over a Go patch.** The plugin API (`docs/plugins.md`) covers timers,
   playback control, keybindings, HTTP, and a persistent store. Anything achievable there should
   not become a core patch.
4. **Don't delete upstream code you merely stopped calling.** Deleting an upstream file guarantees
   conflicts later. `upgrade/` is intentionally left in place but unreferenced (see below).

## What the rebrand changed

Only the user-visible surface. Functional renames, not cosmetic ones:

| Area | cliamp | grbfy | Why it matters |
|---|---|---|---|
| Config dir | `~/.config/cliamp` | `~/.config/grbfy` | The IPC socket lives inside the config dir, so sharing it would make the two players fight over one socket. |
| Data dir | `~/.local/share/cliamp` | `~/.local/share/grbfy` | |
| Save dir | `~/Music/cliamp` | `~/Music/grbfy` | |
| Env override | `CLIAMP_CONFIG_DIR` | `GRBFY_CONFIG_DIR` | |
| IPC socket | `cliamp.sock` | `grbfy.sock` | Both can run side by side. |
| URI scheme | `cliamp://` | `grbfy://` | Two constants: `internal/deeplink.Scheme` **and** `cmd.SchemeName`. Keep them in sync. |
| MPRIS bus | `org.mpris.MediaPlayer2.cliamp` | `...MediaPlayer2.grbfy` | Avoids a D-Bus name collision when both run. |
| Desktop handler | `cliamp-url-handler.desktop` | `grbfy-url-handler.desktop` | |
| Wordmark | `CLIAMP` pixel logo | `GRBFY` | `ui/vis_logo.go` — 5×7 bitmaps, now 5 letters instead of 6. |
| Binary | `cliamp` | `grbfy` | `Makefile` |

**Deliberately NOT renamed:**

- The Go module path (rule 1).
- `radio.cliamp.stream` stream URLs — real external services.
- The `cliamp-plugin-<name>` install convention (`pluginmgr/resolve.go`) — keeps community plugins
  installable.
- **The `cliamp` Lua global.** `luaplugin/luaplugin.go` binds *both* `grbfy` and `cliamp` to the
  same API table. Every existing plugin calls `cliamp.*`; breaking that would break the bundled
  plugins in `plugins/` and the whole community ecosystem for no gain.

### `grbfy upgrade` is disabled

Upstream's updater downloads `bjarneo/cliamp` release binaries, which would silently replace grbfy
with a different program. The command now returns an error instead. `upgrade/` remains on disk,
unreferenced, per rule 4.

## Features added by this fork

1. **Shuffle you can see** — upstream computes a shuffled `order []int`
   (`playlist/playlist.go:360`) that no UI ever exposes. `playlist/upcoming.go` adds
   `UpcomingWindow`, resolving what will actually play (queue first, then the order)
   by the same rules as `Next`, and reporting when a shuffle wrap means the next order
   has not been drawn yet. `ui/model/upnext.go` shows it on <kbd>U</kbd>;
   `playlist/reshuffle.go` re-rolls the order on <kbd>Z</kbd> without disturbing the
   current track.
2. **Study mode** — `plugins/sleep-timer.lua` (<kbd>W</kbd>) and `plugins/pomodoro.lua`
   (<kbd>F</kbd>). Pure Lua, no Go changes. See `docs/study-mode.md`.
3. **Spotify album art** — `AlbumArtURL` is now populated from the `images` already
   present in every Spotify track object (`external/spotify/provider_shared.go`), so it
   costs no extra request. Upstream only populated it for the `local` and `mixcloud`
   providers. This feeds desktop notifications and MPRIS, which render real images.
4. **The transport survives overlays** — upstream lets every overlay swallow the
   whole keyboard, so opening a list costs you play/pause and skip.
   `ui/model/keys_transport.go` adds one `transportKey` handler, called from the
   **`default:` branch** of each overlay's key switch. That ordering is the whole
   design: the overlay's own cases match first, so Space still marks a file in the
   browser and `.` still jumps it to the working directory — only keys the overlay
   ignores fall through. A pre-dispatch intercept would need a hand-maintained
   conflict list that rots silently. Text-entry modes are never reached. Plugin
   keys ride the same fallback, so <kbd>W</kbd> and <kbd>F</kbd> work anywhere.
5. **A themed pomodoro clock** — the image clock's colour has to live in the
   pixels: a kitty placeholder cell spends its foreground colour carrying the
   image id, so the terminal cannot tint the digits. `ui/kittyclock.go` therefore
   rasterizes the glyphs once as white masks and re-tints and re-encodes only per
   colour, off the render goroutine — otherwise stepping through the theme picker
   would stall on a PNG encode per keystroke. Default theme stays white.
6. **Spotify playlist folders** — the public Web API's `/v1/me/playlists` has no
   folder concept at all, so upstream (and grbfy until now) showed every playlist
   flattened under ownership-based buckets ("Your playlists" / "Followed
   playlists"). `external/spotify/rootlist.go` reads the same private "rootlist"
   resource (`hm://playlist/v2/user/<user>/rootlist`) Spotify's own apps use for
   folders, over the AP/Mercury/spclient connection go-librespot already opens
   for playback — no new auth flow or scope, since that connection is
   authenticated via the shared keymaster client's "streaming" token regardless
   of which client_id is configured for the Web API leg (`session.go`'s
   `playbackOAuthScopes` comment). Folders are plain "start-group"/"end-group"
   marker entries bracketing playlist entries in an otherwise flat, undocumented
   protobuf list — nesting is just bracket depth. This is not part of any
   documented Spotify API and could change or break without notice; failure is
   logged and swallowed (`rootlistFolders`), falling back to the old flat
   Section grouping rather than breaking playlist loading.
7. **Folders play as one queue** — a folder is not just a heading: each one gets
   a synthetic `spotify:folder:<path>` row that `Tracks()` expands into the
   concatenated tracks of every playlist inside it, subfolders included
   (`folderTracks`). It rides the ordinary "select row → Tracks(ID) → load"
   path that saved albums already use, so no UI code was involved. One child
   failing (region-locked, deleted) is skipped with a warning rather than
   sinking the whole folder.
8. **Spotify artist browsing** — Spotify was the only configured provider that
   implemented neither `ArtistBrowser` nor `TrackArtistResolver`, so the `N`
   browser other providers share had nothing to show for it: no discography, no
   "jump to this track's artist". `external/spotify/artist_browse.go` adds both
   plus a `BrowseEntryProvider` shortcut. No UI changes — the browser detects
   capability by type assertion (`docs/provider-development.md` is explicit
   about not touching it). Two things are worth remembering here: `Artists()`
   pages by **cursor** (`/v1/me/following` is the only Spotify endpoint in this
   package that does, everything else is limit/offset), and both methods must
   paginate to completion internally because the UI calls them exactly once and
   treats the result as final. `ArtistForTrack` does no I/O — the artist ID is
   stashed in `ProviderMeta` when the track is parsed, which is why the
   `fields=` projection in `Tracks()` now asks for `artists(id,name)`.
   Both `Artists()` and `ArtistAlbums()` hit the exact Development Mode
   quota bug this fork already fixed once for `/v1/search`
   (`dc5eaee`, `isInvalidLimit`/`devModeSearchLimit` in `provider.go`):
   `/v1/me/following` and `/v1/artists/{id}/albums` are catalog endpoints
   too, and reject a limit above 10 with the same misleading `400 "Invalid
   limit"`. Confirmed against a real account, not theoretical — a user hit
   it immediately on first use. Both methods now try the full page size
   first and fall back to `devModeSearchLimit` only on rejection, reusing
   the existing helper rather than duplicating the detection logic.
9. **A lyrics visualizer** — `ui/vis_lyrics.go` draws the current synced lyric
   line big, as pixel-art letters, reusing `vis_logo.go`'s Braille-bitmap
   technique (hand-authored 5×7 glyphs, stamped into a dot grid, rendered as
   Braille) rather than plain text — a fullscreen (`V`) karaoke line is meant
   to read at a glance. A scale search picks the largest size that still lets
   the line wrap within a few rows for the current panel size, and falls back
   to small centered text whenever there is no synced line (radio streams, no
   LRCLIB/NetEase match, an instrumental intro before the first line) or the
   line uses a character outside the hand-authored glyph set — most notably
   any non-Latin script. There is no realistic way to hand-author a 5×7
   bitmap for Devanagari conjuncts, so those lyrics fall back to normal-size
   text rather than drawing gaps; a `DECDHL` terminal-native double-height
   escape was considered instead (would work for any script) but rejected
   without testing, since it's the same class of trick that already failed
   for album art (see below) — Bubbletea's renderer doesn't pass raw escapes
   through untouched, and there's no way to verify it from here.
   This is Go rather than a Lua plugin, against rule 3, deliberately: the Lua
   visualizer API exposes playback position and track metadata but **no**
   lyrics, so a plugin would have to re-implement fetching, LRC parsing and
   caching from scratch and keep its state separate from the app's. The
   active-line scan the `y` overlay had inline is now `lyrics.ActiveLineIndex`,
   shared by both. The one intrusion into upstream state is a `Lyrics` field on
   `VisTickContext`, filled once per tick by `visualizerLyricsContext` — the
   driver never reaches into player or lyrics state itself.
10. **An unavailable track skips instead of demanding sign-in** — `isAuthError`
    treated *every* `audio.KeyProviderError` as a session failure, so Spotify
    refusing the AES key for one region-locked or pulled track triggered a full
    reconnect and then the sign-in prompt. Key code 2 (`aesKeyErrUnavailable`)
    means "this account cannot play this track" and reconnecting cannot fix it,
    so it now surfaces as `playlist.ErrTrackUnavailable` and
    `ui/model/unavailable.go` marks the track `Unplayable` and advances. The
    mark is what bounds the recursion: `Playlist.Next` skips unplayable
    entries, so a queue of dead tracks winds down instead of looping on one.
    Rare before autoplay; constant after it, since Last.fm suggestions resolve
    to whatever Spotify search returns, region locks included.

11. **Radio is a context, not a habit** — playing a track straight from search
    is a different intent from opening a playlist, so `Playlist.PlayNow`
    (`playlist/playnow.go`) drops the old list's remaining `order` while
    keeping anything `q`-queued, and marks the playlist `radio`. `Replace`
    clears that mark, so loading any real list ends radio mode. The flag
    reaches plugins as `grbfy.player.radio()`, which is what stops
    `autoplay.lua` from piling Last.fm picks onto a playlist that already
    says what plays next — seeded, worse, from whatever played before the
    switch. `c` clears the play-next queue from the main view for the times
    you want it gone anyway.

### Album art in the TUI was tried and removed

A `Cover` visualizer drew the art as half-blocks. It was cut because the result is
inherently poor, not because of a fixable bug — worth recording so nobody rebuilds it:

- A character cell can carry at most **two independently coloured pixels** (the halves
  of `▀`). Quadrant blocks add shapes but not colours; Braille adds dots but is
  monochrome. So a fullscreen cover tops out around **60×40 pixels** — a few percent of
  a 300px cover, which reads as a blurry mosaic.
- Kitty graphics would fix it, and ghostty supports it. **It was tested**: the same
  escape sequence draws a sharp image written straight to the terminal, and draws
  nothing when emitted from inside a Bubbletea view. Bubbletea renders through
  `ultraviolet`, a cell-based diffing renderer with no image support, and `Program`
  exposes no positioned raw write (`Println`/`Printf` go to scrollback;
  `ReleaseTerminal` is a full suspend).
- The only remaining route is writing image escapes to the tty concurrently with
  Bubbletea's own writer, re-emitting blindly on a timer since there is no repaint
  hook. That races the renderer and smears on resize. Not worth it for one visualizer.

Ideas deliberately left for later, with the research behind them, are in the plan at
`~/.claude/plans/`. The largest is Spotify Connect device presence: upstream builds a
go-librespot session for streaming but never runs the dealer/connectstate loop, so
other Spotify clients cannot see or control it.

## Everything else

Architecture, build commands, provider contracts, and conventions are unchanged from upstream —
see the upstream README and `docs/`. Build with `make build` (needs Go 1.26+ and `alsa-lib`),
test with `make test`, and run `make check` before finishing a change.
