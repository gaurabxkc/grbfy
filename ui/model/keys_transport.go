package model

import (
	tea "charm.land/bubbletea/v2"
)

// transportKey handles the playback controls that keep working while an
// overlay is open. An overlay is a view onto the music, not a modal worth
// losing the transport over: `.` in the queue should skip the track exactly as
// it does in the main view.
//
// It is called from the *default* branch of each overlay's key switch, so an
// overlay that binds one of these keys for itself always wins — Space still
// marks a file in the browser, and `.` still jumps it to the working
// directory. Only keys the overlay ignores reach here.
//
// Bare Left/Right seeking is deliberately absent: overlays navigate with the
// arrow keys. Shift+Left/Right seek instead, and are unbound everywhere else.
func (m *Model) transportKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "space":
		cmd := m.togglePlayPause()
		m.notifyPlayback()
		return cmd

	case ">", ".":
		refresh := m.scrobbleCurrent()
		cmd := m.nextTrack()
		m.notifyPlayback()
		return tea.Batch(refresh, cmd)

	case "<", ",":
		refresh := m.scrobbleCurrent()
		cmd := m.prevTrack()
		m.notifyPlayback()
		return tea.Batch(refresh, cmd)

	case "shift+left":
		return m.doSeek(-m.seekStepLarge)

	case "shift+right":
		return m.doSeek(m.seekStepLarge)

	case "+", "=":
		m.player.SetVolume(m.player.Volume() + 1)
		m.notifyPlayback()

	case "-":
		m.player.SetVolume(m.player.Volume() - 1)
		m.notifyPlayback()

	default:
		// Plugin bindings are global too: a pomodoro or sleep-timer key is
		// about the session, not about whichever list happens to be open.
		if m.luaMgr != nil {
			m.luaMgr.EmitKey(msg.String())
		}
	}
	return nil
}
