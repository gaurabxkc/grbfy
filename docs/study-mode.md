# Study mode

Two bundled Lua plugins for working to music: a **sleep timer** that fades out
and stops, and a **pomodoro** timer that pauses the music during breaks.

Both are plugins rather than core features, they need no changes to grbfy
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

Press <kbd>Ctrl+Y</kbd> to cycle: off → 15m → 30m → 45m → 60m → off.

When the time is up the volume fades down over 20 seconds, playback stops, and
**the volume is restored**, so the fade does not quietly follow you into the
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

Press <kbd>H</kbd> to start a focus session, and <kbd>H</kbd> again to stop it.
While it runs, <kbd>)</kbd> adds a minute to the current phase and <kbd>(</kbd>
takes one off (never below one minute). Only the running phase changes; the next
one uses its configured length.

The key used to be <kbd>F</kbd>. Upstream cliamp took `F` for its subscribed
shows overlay, and core keys are reserved from plugins.

Music plays through the work phase and **pauses for breaks**, so the silence is
what marks the break, no timer to watch. After four rounds the break is a long
one. Each transition shows a status-bar message and a desktop notification.

If you pause the music yourself during a break, the plugin leaves it paused: it
only resumes playback it paused itself.

Completed rounds are counted in the plugin's persistent store, so the all-time
total survives restarts and shows up in `status`.

### The big countdown

The plugin also registers a **visualizer** called `pomodoro`. Press <kbd>v</kbd> to
cycle to it, or <kbd>Ctrl+V</kbd> and pick it, and the visualizer band becomes a
full-width block-digit countdown, the clock and nothing else. <kbd>V</kbd> makes
it full screen.

The digits are drawn from curves at the size the panel gives them, rather than
a small bitmap font scaled up, and each character cell carries two pixels
(`▀ ▄ █`) for twice the vertical resolution, so the clock stays smooth instead
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

The clock takes the **theme's accent colour**, the same colour as the seek bar
and the title, so it changes with <kbd>t</kbd>. On the default theme it stays
white, because that palette is the terminal's own ANSI colours, which say
nothing dependable about what will read well as a shape the size of the panel.

Colouring the image clock means re-sending the glyphs: a placeholder cell spends
its foreground colour carrying the image id, so the terminal cannot tint the
digits. The outlines are rasterized once and only re-tinted and re-encoded per
colour, off the render goroutine, so stepping through the theme picker costs the
clock a frame in the old colour rather than a stall.

```toml
[plugins.pomodoro]
cell_aspect = 3.0        # cell height ÷ width; 2.0 is typical, higher for a
                         # narrow font or extra line spacing
clock_font = "semicondensed"   # "bold" | "semicondensed" | "condensed"
clock_stretch = 1.45           # how far past the font's proportions it may grow
```

**Both settings are really height controls.** Four digits abreast make the clock
width-limited, so once it spans the width it cannot grow taller without either a
narrower cut or some stretching. `clock_font` picks the cut, `bold` is the
widest letterform, `condensed` the narrowest and so the tallest. `clock_stretch`
allows the digits to grow narrower than the typeface intends in order to use the
panel's height; `1.0` keeps the proportions exactly and accepts the blank rows.

This works because a visualizer plugin still gets hooks, keybindings and
commands, the type only adds `render` on top.

```sh
grbfy plugins call pomodoro start
grbfy plugins call pomodoro status   # phase, time left, all-time count
grbfy plugins call pomodoro stop
```

```toml
[plugins.pomodoro]
work_minutes = 90
break_minutes = 5
long_break_minutes = 15
rounds_before_long_break = 4
adjust_minutes = 1          # step for ( and )
```

## Keys

<kbd>Ctrl+Y</kbd>, <kbd>H</kbd>, <kbd>(</kbd> and <kbd>)</kbd> are bound at load time and appear in the
<kbd>Ctrl+K</kbd> keymap under ",  plugins , ". They work from any list-style
overlay as well as the main view, because a timer key is about the session
rather than about whichever list happens to be open; a key the overlay binds
for itself still wins. Text input is never intercepted. If a future grbfy
release claims either key for the core UI, the binding is refused with a
warning in `~/.config/grbfy/plugins.log` and the shell commands above still
work.
