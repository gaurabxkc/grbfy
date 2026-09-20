package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serveCover publishes a solid-colour JPEG of the given size, standing in for
// a provider's artwork.
func serveCover(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0x40, 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/cover.jpg"
}

// waitForCover waits for the background fetch to decode the artwork.
func waitForCover(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		coverMu.Lock()
		got := coverImg != nil
		coverMu.Unlock()
		if got {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cover was never fetched")
}

// resetCover clears the cached artwork so each test starts cold.
func resetCover(t *testing.T) {
	t.Helper()
	reset := func() {
		coverMu.Lock()
		coverURL, coverImg, coverFetched, coverSentURL = "", nil, "", ""
		coverMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestRenderCoverTransmitsAndPlaces(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	out := capturePlacements(t)

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	const rows, cols = 12, 80
	got, ok := RenderCover(rows, cols)
	if !ok {
		t.Fatal("RenderCover reported nothing to draw")
	}

	sent := out.String()
	if !strings.Contains(sent, "a=t") {
		t.Error("the image was never transmitted")
	}
	// Deleting the old image drops its placements, so a placement must follow
	// the transmission, not precede it.
	del := strings.LastIndex(sent, "a=d")
	if del < 0 || !strings.Contains(sent[del:], "a=p") {
		t.Error("the image was not placed after being sent")
	}

	lines := strings.Split(got, "\n")
	if len(lines) != rows {
		t.Errorf("render has %d lines, want %d", len(lines), rows)
	}
	placeholders := 0
	for i, line := range lines {
		if n := cellWidth(ansiRe.ReplaceAllString(line, "")); n > cols {
			t.Errorf("line %d is %d cells wide, want <= %d", i, n, cols)
		}
		placeholders += strings.Count(line, string(rune(0x10EEEE)))
	}
	if placeholders == 0 {
		t.Error("no placeholder cells in the render")
	}
}

// A square cover must come out square on screen: cells are taller than they
// are wide, so it needs more columns than rows.
func TestCoverBoxKeepsProportions(t *testing.T) {
	square := image.NewRGBA(image.Rect(0, 0, 300, 300))

	rows, cols := coverBox(square, 10, 80)
	if want := int(float64(rows)*coverCellAspect + 0.5); cols != want {
		t.Errorf("box is %dx%d cells; a square needs %d columns for %d rows", cols, rows, want, rows)
	}

	// A narrow panel limits the columns, and the rows must shrink with them.
	rows, cols = coverBox(square, 20, 12)
	if cols > 12 {
		t.Errorf("box is %d columns wide, want <= 12", cols)
	}
	if rows >= 20 {
		t.Errorf("rows did not shrink with the width: %d", rows)
	}
}

// Without artwork, or on a terminal with no graphics protocol, the caller has
// to be told so it can fall back to text.
func TestRenderCoverWithoutArt(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)

	if _, ok := RenderCover(12, 80); ok {
		t.Error("reported a cover with no artwork set")
	}

	SetCoverArt(serveCover(t, 300, 300))
	waitForCover(t)
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "0")
	if _, ok := RenderCover(12, 80); ok {
		t.Error("drew an image on a terminal without graphics support")
	}
}

// A track change replaces the artwork rather than leaving the last one up.
func TestSetCoverArtReplacesOnTrackChange(t *testing.T) {
	resetCover(t)

	SetCoverArt(serveCover(t, 300, 300))
	waitForCover(t)

	SetCoverArt(serveCover(t, 200, 200))
	coverMu.Lock()
	stale := coverImg
	coverMu.Unlock()
	if stale != nil {
		t.Error("the previous cover survived a track change")
	}
	waitForCover(t)
}

// cellWidth counts display cells: a placeholder is one cell carrying two
// combining marks, which occupy no width of their own.
func cellWidth(s string) int {
	marks := map[rune]bool{}
	for _, r := range rowColumnDiacritics {
		marks[r] = true
	}
	n := 0
	for _, r := range s {
		if !marks[r] {
			n++
		}
	}
	return n
}
