# Study mode

Two bundled Lua plugins for working to music: a **sleep timer** that fades out
and stops, and a **pomodoro** timer that pauses the music during breaks.

Both are plugins rather than core features — they need no changes to grbfy
itself, and you can edit their behavior without rebuilding.

## Install

Plugins ship in the repo but are not active until you copy and trust them:

```sh
mkdir -p ~/.config/grbfy/plugins
cp plugins/sleep-timer.lua plugins/pomodoro.lua ~/.config/grbfy/plugins/
grbfy plugins trust sleep-timer
grbfy plugins trust pomodoro
```

Trust is by file contents: editing a plugin disables it until you approve it
again. Confirm with `grbfy plugins list`.

## Sleep timer

Press <kbd>W</kbd> to cycle: off → 15m → 30m → 45m → 60m → off.

When the time is up the volume fades down over 20 seconds, playback stops, and
**the volume is restored** — so the fade does not quietly follow you into the
next session. You get a desktop notification one minute before the end.

From a shell:

```sh
grbfy plugins call sleep-timer set 45
grbfy plugins call sleep-timer status
grbfy plugins call sleep-timer cancel
```

```toml
[plugins.sleep-timer]
fade_seconds = 20
```

## Pomodoro

Press <kbd>F</kbd> to start a focus session, and <kbd>F</kbd> again to stop it.

Music plays through the work phase and **pauses for breaks**, so the silence is
what marks the break — no timer to watch. After four rounds the break is a long
one. Each transition shows a status-bar message and a desktop notification.

If you pause the music yourself during a break, the plugin leaves it paused: it
only resumes playback it paused itself.

Completed rounds are counted in the plugin's persistent store, so the all-time
total survives restarts and shows up in `status`.

### The big countdown

The plugin also registers a **visualizer** called `pomodoro`. Press <kbd>v</kbd> to
cycle to it, or <kbd>Ctrl+V</kbd> and pick it, and the visualizer band becomes a
full-width block-digit countdown — the clock and nothing else. <kbd>V</kbd> makes
it full screen.

The digits are drawn from curves at the size the panel gives them, rather than
a small bitmap font scaled up, and each character cell carries two pixels
(`▀ ▄ █`) for twice the vertical resolution — so the clock stays smooth instead
of turning blocky as it grows. A thin line beneath shows progress through the
phase: elapsed bright, remaining dimmed. Nothing else is drawn; the phase is
announced in the status bar and by the music itself pausing.

**Size follows height**, since the digits keep their proportions. A 10-row band
gives a clock about 78 cells wide; 20 rows gives about 160. So raise `vis_rows`
in `config.toml` if you want it bigger inline, and press <kbd>V</kbd> for the
whole window. Below about 6 rows it falls back to plain text.

On a terminal that supports the kitty graphics protocol (ghostty, kitty) the
digits are drawn as **real images at screen resolution** rather than block
characters, so they are genuinely smooth. Elsewhere the block rendering is used.

```toml
[plugins.pomodoro]
cell_aspect = 3.0        # cell height ÷ width; 2.0 is typical, higher for a
                         # narrow font or extra line spacing
clock_font = "semicondensed"   # "bold" | "semicondensed" | "condensed"
clock_stretch = 1.45           # how far past the font's proportions it may grow
```

**Both settings are really height controls.** Four digits abreast make the clock
width-limited, so once it spans the width it cannot grow taller without either a
narrower cut or some stretching. `clock_font` picks the cut — `bold` is the
widest letterform, `condensed` the narrowest and so the tallest. `clock_stretch`
allows the digits to grow narrower than the typeface intends in order to use the
panel's height; `1.0` keeps the proportions exactly and accepts the blank rows.

This works because a visualizer plugin still gets hooks, keybindings and
commands — the type only adds `render` on top.

```sh
grbfy plugins call pomodoro start
grbfy plugins call pomodoro status   # phase, time left, all-time count
grbfy plugins call pomodoro stop
```

```toml
[plugins.pomodoro]
work_minutes = 25
break_minutes = 5
long_break_minutes = 15
rounds_before_long_break = 4
```

## Keys

<kbd>W</kbd> and <kbd>F</kbd> are bound at load time and appear in the
<kbd>Ctrl+K</kbd> keymap under "— plugins —". Plugin keys work only in the main
view; overlays capture their own input. If a future grbfy release claims either
key for the core UI, the binding is refused with a warning in
`~/.config/grbfy/plugins.log` and the shell commands above still work.
