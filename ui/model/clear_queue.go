package model

import (
	tea "charm.land/bubbletea/v2"
)

// toggleQueueAtCursor adds the track under the playlist cursor to the
// play-next queue, or removes it if it is already queued. Bound to both a and
// q in the playlist pane: q is what queues in the search overlays, so it
// means the same thing here rather than quitting.
func (m *Model) toggleQueueAtCursor() tea.Cmd {
	if !m.playlist.Dequeue(m.plCursor) {
		m.playlist.Queue(m.plCursor)
	}
	m.normalizeQueueOverlay()
	return m.rearmPreload()
}

// clearPlayNextQueue empties the play-next queue from the main view, so
// dropping everything autoplay piled up doesn't require opening the queue
// manager with A first.
//
// Snapshots before clearing so Ctrl+Z restores it, matching the manager's own
// c binding rather than inventing a second, less forgiving behaviour.
func (m *Model) clearPlayNextQueue() tea.Cmd {
	if m.playlist.QueueLen() == 0 {
		m.status.Show("Queue is already empty", statusTTLShort)
		return nil
	}
	m.playlistUndo = playlistUndo{active: true, snapshot: m.playlist.Snapshot()}
	m.playlist.ClearQueue()
	m.normalizeQueueOverlay()
	m.status.Show("Cleared queue (Ctrl+Z to undo)", statusTTLDefault)
	return m.rearmPreload()
}
