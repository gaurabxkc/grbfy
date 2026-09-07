package luaplugin

import (
	"testing"
	"time"
)

// A plugin that is both the active visualizer and handling an event holds its
// Lua lock in a hook goroutine while calling back into the UI — and those calls
// block until the UI loop consumes them. If RenderVis waited for that lock, the
// UI loop and the hook would deadlock and the program would freeze.
func TestRenderVisDoesNotBlockOnBusyPlugin(t *testing.T) {
	m := newManagerWithPlugin(t, "vis", `
		local p = plugin.register({name = "vis", type = "visualizer"})
		function p:render(bands, frame, rows, cols)
			return "frame"
		end
	`)

	var bands [10]float64
	if got := m.RenderVis("vis", bands, 4, 20, 0); got != "frame" {
		t.Fatalf("first render = %q, want %q", got, "frame")
	}

	// Simulate a hook holding the Lua state, as invokeHook does.
	m.mu.RLock()
	vis := m.visMap["vis"]
	m.mu.RUnlock()
	if vis == nil {
		t.Fatal("visualizer not registered")
	}
	vis.plugin.mu.Lock()

	done := make(chan string, 1)
	go func() { done <- m.RenderVis("vis", bands, 4, 20, 1) }()

	select {
	case got := <-done:
		if got != "frame" {
			t.Errorf("busy render = %q, want the previous frame", got)
		}
	case <-time.After(2 * time.Second):
		vis.plugin.mu.Unlock()
		t.Fatal("RenderVis blocked on a busy plugin: this is the UI freeze")
	}
	vis.plugin.mu.Unlock()
}
