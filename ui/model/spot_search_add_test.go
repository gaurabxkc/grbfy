package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/playlist"
	"github.com/bjarneo/cliamp/ui"
)

func spotSearchAddTestModel(t *testing.T) Model {
	t.Helper()
	player := &playbackFakeEngine{}
	m := Model{
		player:   player,
		playlist: playlist.New(),
		provider: commandsTestProvider{name: "Spotify"},
		vis:      ui.NewVisualizer(float64(player.SampleRate())),
	}
	m.spotSearch = spotSearchState{
		prov:    commandsTestProvider{name: "Spotify"},
		visible: true,
		screen:  spotSearchResults,
		query:   "raga",
		results: []playlist.Track{
			{Path: "spotify:track:one", Title: "Dhyaan Se", Artist: "Raga"},
			{Path: "spotify:track:two", Title: "Bada Ghar", Artist: "Raga"},
			{Path: "spotify:track:three", Title: "Pow!", Artist: "Bhaktaaa"},
		},
	}
	return m
}

// Queueing a song from the results must leave them open, so a second and a
// third can be picked from the same search.
func TestSpotSearchQueueKeepsTheResultsOpen(t *testing.T) {
	for _, key := range []string{"q", "a"} {
		t.Run(key, func(t *testing.T) {
			m := spotSearchAddTestModel(t)

			m.handleSpotSearchResultsKey(tea.KeyPressMsg{Text: key})
			m.spotSearch.cursor = 2
			m.handleSpotSearchResultsKey(tea.KeyPressMsg{Text: key})

			if !m.spotSearch.visible {
				t.Fatal("the results closed after adding a song")
			}
			if m.playlist.Len() != 2 {
				t.Fatalf("playlist has %d tracks, want the 2 added", m.playlist.Len())
			}
			if len(m.spotSearch.results) != 3 {
				t.Fatalf("results changed to %d entries", len(m.spotSearch.results))
			}
			if !m.spotSearch.added["spotify:track:one"] || !m.spotSearch.added["spotify:track:three"] {
				t.Fatalf("added = %v, want both picks marked", m.spotSearch.added)
			}
			if m.spotSearch.added["spotify:track:two"] {
				t.Fatal("a song that was not added is marked")
			}
		})
	}
}

// Enter plays the song now, which is the end of the search.
func TestSpotSearchPlayStillClosesTheResults(t *testing.T) {
	m := spotSearchAddTestModel(t)
	m.handleSpotSearchResultsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.spotSearch.visible {
		t.Fatal("the results stayed open after playing a song")
	}
}

// What has been added shows in the list, so a pick is not made twice.
func TestSpotSearchMarksAddedRows(t *testing.T) {
	withFrameWidth(t, 100)
	m := spotSearchAddTestModel(t)
	m.spotSearch.markSpotAdded("spotify:track:two")

	out := m.renderSpotSearchResults(10)
	var marked, unmarked int
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "Bada Ghar"):
			if !strings.Contains(line, "✓") {
				t.Fatalf("added row is not marked: %q", line)
			}
			marked++
		case strings.Contains(line, "Dhyaan Se"), strings.Contains(line, "Pow!"):
			if strings.Contains(line, "✓") {
				t.Fatalf("row that was not added is marked: %q", line)
			}
			unmarked++
		}
	}
	if marked != 1 || unmarked != 2 {
		t.Fatalf("saw %d marked and %d unmarked rows in:\n%s", marked, unmarked, out)
	}
}
