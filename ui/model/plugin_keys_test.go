package model

import "testing"

// TestStudyPluginKeysStayFree fails when an upstream merge claims a key the
// study-mode plugins bind. Reserved keys are refused to plugins, so the
// binding would otherwise just go dead — which is how the pomodoro lost F.
func TestStudyPluginKeysStayFree(t *testing.T) {
	reserved := ReservedKeys()
	for key, plugin := range map[string]string{
		"W": "sleep-timer",
		"H": "pomodoro",
		"(": "pomodoro",
		")": "pomodoro",
	} {
		if reserved[key] {
			t.Errorf("core now reserves %q, which %s binds; move the plugin to a free key", key, plugin)
		}
	}
}
