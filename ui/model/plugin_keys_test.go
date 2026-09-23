package model

import "testing"

// TestStudyPluginKeysStayFree fails when an upstream merge claims a key one of
// my plugins binds. Reserved keys are refused to plugins, so the binding would
// otherwise just go dead — which is how the pomodoro lost F and autoplay lost
// ctrl+a. autoplay lives only in ~/.config/grbfy/plugins, not in this repo.
func TestStudyPluginKeysStayFree(t *testing.T) {
	reserved := ReservedKeys()
	for key, plugin := range map[string]string{
		"ctrl+y": "sleep-timer",
		"H":      "pomodoro",
		"(":      "pomodoro",
		")":      "pomodoro",
		"ctrl+t": "autoplay",
	} {
		if reserved[key] {
			t.Errorf("core now reserves %q, which %s binds; move the plugin to a free key", key, plugin)
		}
	}
}
