package model

import (
	"strings"
	"testing"

	"github.com/bjarneo/cliamp/ui"
)

func TestFullVisQueueTogglesAndKeepsHeight(t *testing.T) {
	m := &Model{}
	if m.fullVisQueue {
		t.Fatal("the fullscreen queue should start hidden")
	}

	m.toggleFullVisQueue()
	if !m.fullVisQueue {
		t.Error("toggleFullVisQueue() did not turn the queue on")
	}
	m.toggleFullVisQueue()
	if m.fullVisQueue {
		t.Error("toggleFullVisQueue() did not turn the queue back off")
	}
}

func TestRenderFullVisQueueFillsExactlyTheVisualizerHeight(t *testing.T) {
	m := Model{}
	m.vis = ui.NewVisualizer(44100)
	m.vis.Rows = 6

	got := m.renderFullVisQueue()
	if n := len(strings.Split(got, "\n")); n != m.vis.Rows {
		t.Errorf("rendered %d rows, want %d — the seek bar below must not shift", n, m.vis.Rows)
	}
}

func TestFullVisQueueHelpLabelsTheOtherState(t *testing.T) {
	m := Model{}
	off := m.fullVisQueueHelp()
	m.fullVisQueue = true
	on := m.fullVisQueueHelp()
	if off == on {
		t.Error("the help label should say what u will do next, not the current state")
	}
}
