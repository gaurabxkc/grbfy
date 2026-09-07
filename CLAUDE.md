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
