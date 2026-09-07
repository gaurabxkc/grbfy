# Roadmap — closing the gap for a Spotify user

The aim is that moving from the Spotify client to grbfy does not mean giving
things up. This lists what already works, what is genuinely missing, and what is
not worth attempting — each checked against the code rather than assumed.

## Already there (do not rebuild)

Worth knowing, because several of these look missing until you find the key:

| Capability | How |
|---|---|
| Play / append / queue-next from search results | `Enter` / `a` / `q` in the results overlay |
| Save a search result to a Spotify playlist | `p` in the results overlay |
| Play-next queue with reordering and removal | `a` toggles, `A` opens the manager |
| See the resolved play order | `U` (Up Next), `Z` re-rolls the shuffle |
| Liked Songs, playlists, saved albums | Spotify provider, `S` |
| Podcasts / episodes | Handled alongside tracks |
| Lyrics, EQ, gapless, resume-on-restart | Core |
| Local favourites (`n`) and bookmarks (`f`) | Stored in `favorites.toml` — **local only**, see below |

**The OAuth scopes already requested** (`external/spotify/session.go`) cover far
more than the code currently uses: `user-library-modify`,
`user-read-recently-played`, `user-top-read`, `user-modify-playback-state`,
`user-follow-modify`. Several items below therefore need only the API call and
the UI — no re-authentication, no new consent screen.

## Phase 1 — Queue control

The Up Next panel is read-only. Spotify's queue is not, and that difference is
felt constantly.

- Cursor in the Up Next panel, `Enter` to jump straight to a track.
- `Shift+Up` / `Shift+Down` to reorder, `d` to drop an entry.
- Show what a change does to the *resolved* order, not just the queue.

**The catch:** upcoming order lives in the playlist's private `order []int`
(`playlist/playlist.go`), not in the visible track list. `Playlist.Move`
reorders the *visual* list. Reordering Up Next means rewriting `order`, so it
needs a new method beside `UpcomingWindow` — with the same rule that the entries
past a shuffle wrap are not yet decided and cannot be moved.

## Phase 2 — Stop lying about Likes

`n` writes to a local `favorites.toml`. A Spotify user pressing it reasonably
expects the track to be Liked in Spotify — it is not, and nothing says so.

- `PUT` / `DELETE /v1/me/tracks` so `n` saves and unsaves for real.
- Show the true saved state on rows, read from the library.
- Keep the local store for non-Spotify providers, and be explicit in the UI
  about which one a heart refers to.
- `DELETE /v1/playlists/{id}/tracks` — playlists can currently be added to
  (`POST .../items`) but never removed from, which makes editing one-way.

Scopes are already granted, so this is API plumbing plus display state.

## Phase 3 — Getting back what you listened to

- **Recently played** — `/v1/me/player/recently-played`. There is already a local
  "Recently Played" list (`history/`), but it only knows what grbfy itself
  played; anything played on the phone is invisible.
- **Top tracks / artists** — `/v1/me/top/tracks`, `/v1/me/top/artists`. New
  provider views, no new plumbing.

## Phase 4 — Spotify Connect presence

The largest gap, and the largest job. grbfy builds a go-librespot session with
`DeviceType_COMPUTER` for streaming, but never runs the dealer/connectstate
loop — so **your phone cannot see or control it**, and playback cannot be handed
between devices. This is table stakes for anyone with more than one device, and
is what `spotify-player` and `spotatui` lead with.

Needs: dealer websocket, publishing device state, and handling remote commands.
Worth doing only as a deliberate project, not squeezed in.

## Not worth attempting

- **Spotify's own recommendations / song radio.** `/v1/recommendations` is
  permanently 403 for any app created after 2024-11-27, with no waitlist. The
  working substitute is Last.fm `track.getSimilar` — which the `autoplay.lua`
  plugin already does.
- **Album art in the terminal.** Tried and removed; see `CLAUDE.md` for why the
  character grid cannot render it well.

## Ordering

Phase 1 first: it is the one felt every session, and it is self-contained.
Phase 2 next — it is cheap and it removes an outright falsehood in the UI.
Phase 3 is small and additive. Phase 4 only when there is appetite for it.
