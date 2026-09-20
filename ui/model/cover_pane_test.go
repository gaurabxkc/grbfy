package model

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/bjarneo/cliamp/playlist"

	"github.com/bjarneo/cliamp/ui"
)

// serveTestCover publishes a square JPEG, gives it to the model's playing
// track, and waits for the fetch so the artwork is ready to draw.
func serveTestCover(t *testing.T, m *Model, width int) {
	t.Helper()
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Cleanup(ui.SetGraphicsOutput(io.Discard))

	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := range 300 {
		for x := range 300 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0x80, 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	m.playlist.SetTrack(0, playlist.Track{
		Title: "Kerala", Artist: "Bonobo", AlbumArtURL: srv.URL + "/cover.jpg",
	})
	ui.SetCoverArt(srv.URL + "/cover.jpg")
	t.Cleanup(func() { ui.SetCoverArt("") })

	deadline := time.Now().Add(5 * time.Second)
	for ui.CoverRowsFor(width) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("cover was never fetched")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The artwork sits above the settings without pushing them out of the pane.
func TestSettingsPaneWithCoverFits(t *testing.T) {
	m := newColumnTestModel(120, 40)
	serveTestCover(t, &m, m.layout.settingsWidth)
	m.SetShowCover(true)

	rows := m.effectivePlaylistVisible()
	pane := m.renderSettingsPaneWithCover(rows)

	if !strings.ContainsRune(pane, 0x10EEEE) {
		t.Error("no album art in the pane")
	}
	if !strings.Contains(ansi.Strip(pane), "Settings") {
		t.Error("the settings separator is missing below the art")
	}
	assertViewFits(t, pane, m.layout.settingsWidth, rows)

	// The controls themselves survive: the volume row carries a dB readout.
	if !strings.Contains(ansi.Strip(pane), "dB") {
		t.Errorf("settings were pushed out of the pane:\n%s", ansi.Strip(pane))
	}
}

// Switched off, the pane is exactly what it was before this feature.
func TestSettingsPaneWithoutCover(t *testing.T) {
	m := newColumnTestModel(120, 40)
	serveTestCover(t, &m, m.layout.settingsWidth)

	rows := m.effectivePlaylistVisible()
	if got, want := m.renderSettingsPaneWithCover(rows), m.renderSettingsPane(rows); got != want {
		t.Error("the pane changed with the cover switched off")
	}
}

// A short pane keeps its controls rather than giving the rows to a picture.
func TestCoverDroppedWhenPaneIsShort(t *testing.T) {
	m := newColumnTestModel(120, 40)
	serveTestCover(t, &m, m.layout.settingsWidth)
	m.SetShowCover(true)

	for _, rows := range []int{0, 1, 4, 6} {
		if got := m.coverPaneRows(rows); got != 0 {
			t.Errorf("pane of %d rows gave %d to the cover; settings need at least %d",
				rows, got, coverPaneMinSettingsRows)
		}
	}
}

// Without artwork there is nothing to draw, whatever the setting says.
func TestCoverPaneWithoutArtwork(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	ui.SetCoverArt("")
	m := newColumnTestModel(120, 40)
	m.SetShowCover(true)

	rows := m.effectivePlaylistVisible()
	if m.coverPaneRows(rows) != 0 {
		t.Error("reserved rows for a cover that does not exist")
	}
	if got, want := m.renderSettingsPaneWithCover(rows), m.renderSettingsPane(rows); got != want {
		t.Error("the pane changed although there is no artwork")
	}
}

// The pane must fetch the artwork itself. Before this, the picture only ever
// arrived if the Cover visualizer happened to be running, so Ctrl+O showed
// nothing until you switched visualizers.
func TestCoverPaneFetchesArtworkWithoutTheVisualizer(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Cleanup(ui.SetGraphicsOutput(io.Discard))
	ui.SetCoverArt("")
	t.Cleanup(func() { ui.SetCoverArt("") })

	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	m := newColumnTestModel(120, 40)
	m.playlist.SetTrack(0, playlist.Track{
		Title: "Kerala", Artist: "Bonobo", AlbumArtURL: srv.URL + "/cover.jpg",
	})
	m.SetShowCover(true)

	rows := m.effectivePlaylistVisible()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.ContainsRune(m.renderSettingsPaneWithCover(rows), 0x10EEEE) {
		if time.Now().After(deadline) {
			t.Fatal("the pane never drew the artwork on its own")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
