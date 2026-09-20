package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
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
		coverURL, coverImg, coverFetching = "", nil, ""
		clear(coverSent)
		clear(coverCache)
		coverCacheKeys = nil
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

// The Cover mode pairs the art with the track's details instead of leaving a
// square picture alone in the middle of the band.
func TestCoverModeDrawsDetailsBesideArt(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Setenv("GRBFY_COVER_CELL_ASPECT", "2")
	coverAspectFixed, coverCellAspect = true, 2
	t.Cleanup(func() { coverAspectFixed = false })
	resetCover(t)
	t.Cleanup(SetGraphicsOutput(io.Discard))

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	defer WithPanelWidth(74)()
	d := &coverDriver{ctx: VisCoverContext{
		TrackTitle:  "Kerala",
		TrackArtist: "Bonobo",
		AlbumLine:   "Migration · 2017",
	}}
	v := &Visualizer{Rows: 10}

	out := d.Render(v)
	lines := strings.Split(out, "\n")
	if len(lines) != v.Rows {
		t.Fatalf("render has %d lines, want %d", len(lines), v.Rows)
	}
	plain := ansiRe.ReplaceAllString(out, "")
	for _, want := range []string{"Kerala", "Bonobo", "Migration · 2017"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q is missing beside the art", want)
		}
	}
	if !strings.ContainsRune(out, 0x10EEEE) {
		t.Error("no album art in the render")
	}
	for i, line := range lines {
		if n := cellWidth(ansiRe.ReplaceAllString(line, "")); n > PanelWidth {
			t.Errorf("line %d is %d cells wide, want <= %d", i, n, PanelWidth)
		}
	}
}

// A panel too narrow for both falls back to the art alone, then to text.
func TestCoverModeNarrowPanel(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Setenv("GRBFY_COVER_CELL_ASPECT", "2")
	coverAspectFixed, coverCellAspect = true, 2
	t.Cleanup(func() { coverAspectFixed = false })
	resetCover(t)
	t.Cleanup(SetGraphicsOutput(io.Discard))

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	defer WithPanelWidth(24)()
	d := &coverDriver{ctx: VisCoverContext{TrackTitle: "Kerala", TrackArtist: "Bonobo"}}
	out := d.Render(&Visualizer{Rows: 10})
	if strings.Contains(ansiRe.ReplaceAllString(out, ""), "Kerala") {
		t.Error("details were drawn into a panel with no room for them")
	}
	if !strings.ContainsRune(out, 0x10EEEE) {
		t.Error("the art itself should still be drawn")
	}
}

// The pane and the visualizer draw the cover at different sizes at the same
// time. Sharing one image id made each render steal the other's placement,
// which showed as the picture flickering between the two sizes.
func TestCoverSlotsDoNotFight(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	out := capturePlacements(t)

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	// First render of each: both transmit and place, once.
	pane, ok := RenderCover(10, 26)
	if !ok {
		t.Fatal("pane cover did not draw")
	}
	block, _, ok := RenderCoverBlock(14)
	if !ok {
		t.Fatal("visualizer cover did not draw")
	}
	if paneID, blockID := placeholderID(t, pane), placeholderID(t, strings.Join(block, "\n")); paneID == blockID {
		t.Fatalf("both covers use image id %d; they need one each", paneID)
	}

	// Steady state: alternating renders must not resend anything.
	out.Reset()
	for range 5 {
		RenderCover(10, 26)
		RenderCoverBlock(14)
	}
	if sent := out.String(); sent != "" {
		t.Errorf("alternating renders re-sent %d bytes: %q", len(sent), sent)
	}
}

// A cover refresh must not blank the other copy: forgetting every placement
// made the pane's cover disappear whenever the visualizer's was re-sent.
func TestCoverRefreshKeepsTheOtherPlacement(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	out := capturePlacements(t)

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)
	RenderCover(10, 26)
	RenderCoverBlock(14)

	// A new track: both copies refresh, each placing itself again.
	SetCoverArt(serveCover(t, 300, 300))
	waitForCover(t)
	out.Reset()
	RenderCover(10, 26)
	RenderCoverBlock(14)

	sent := out.String()
	for _, id := range []int{coverImageID, coverBlockImageID} {
		place := fmt.Sprintf("a=p,U=1,i=%d", id)
		if !strings.Contains(sent, place) {
			t.Errorf("image %d was re-sent but never placed again", id)
		}
	}

	// And the frame after settles: nothing more goes to the terminal.
	out.Reset()
	RenderCover(10, 26)
	RenderCoverBlock(14)
	if extra := out.String(); extra != "" {
		t.Errorf("still writing escapes once settled: %q", extra)
	}
}

// placeholderID reads the image id back out of a rendered cover: it travels in
// the foreground colour of the placeholder cells.
func placeholderID(t *testing.T, render string) int {
	t.Helper()
	m := regexp.MustCompile(`\x1b\[38;2;(\d+);(\d+);(\d+)m`).FindStringSubmatch(render)
	if m == nil {
		t.Fatal("no placeholder colour in the render")
	}
	id := 0
	for _, part := range m[1:] {
		n := 0
		for _, r := range part {
			n = n*10 + int(r-'0')
		}
		id = id<<8 | n
	}
	return id
}

// The image is scaled to the box before it is sent, so the terminal has no
// fitting of its own to do. A different box means a fresh scale.
func TestCoverRescalesWhenTheBoxChanges(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	out := capturePlacements(t)

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	if _, ok := RenderCover(10, 26); !ok {
		t.Fatal("cover did not draw")
	}
	out.Reset()
	if _, ok := RenderCover(10, 26); !ok {
		t.Fatal("cover did not draw")
	}
	if resent := out.String(); resent != "" {
		t.Errorf("same box re-sent the image: %q", resent)
	}

	// A resize changes the box, which has to be re-scaled and re-placed.
	out.Reset()
	if _, ok := RenderCover(6, 26); !ok {
		t.Fatal("cover did not draw after the resize")
	}
	sent := out.String()
	if !strings.Contains(sent, "a=t") {
		t.Error("a smaller box did not re-send the image")
	}
	if !strings.Contains(sent, "a=p") {
		t.Error("a smaller box did not place the image again")
	}
}

// Without the terminal's cell size there is nothing to scale to, and the
// artwork must still be sent as it is rather than dropped.
func TestFitToCellBoxWithoutCellSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 300))
	if got := fitToCellBox(src, 20, 10); got != image.Image(src) {
		// Tests do not run on a terminal, so terminalCellPixels reports
		// nothing and the picture comes back untouched.
		t.Errorf("image was altered without a known cell size: %v", got.Bounds())
	}
}

// Pausing can briefly leave no playing track, which clears the artwork. The
// same URL coming back must draw again: it used to be remembered as already
// fetched, so the cover stayed blank until the track changed.
func TestCoverReturnsAfterBeingCleared(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	t.Cleanup(SetGraphicsOutput(io.Discard))

	url := serveCover(t, 300, 300)
	SetCoverArt(url)
	waitForCover(t)

	SetCoverArt("") // the blip
	if _, ok := RenderCover(10, 26); ok {
		t.Fatal("drew a cover with none set")
	}

	SetCoverArt(url)
	coverMu.Lock()
	restored := coverImg != nil
	coverMu.Unlock()
	if !restored {
		t.Fatal("the cover did not come back from the cache")
	}
	if _, ok := RenderCover(10, 26); !ok {
		t.Error("the cover did not draw again after coming back")
	}
}

// A fetch that fails must not block later attempts at the same artwork.
func TestCoverRetriesAfterAFailedFetch(t *testing.T) {
	resetCover(t)

	var fail atomic.Bool
	fail.Store(true)
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	url := srv.URL + "/cover.jpg"

	SetCoverArt(url)
	deadline := time.Now().Add(2 * time.Second)
	for {
		coverMu.Lock()
		inFlight := coverFetching
		coverMu.Unlock()
		if inFlight == "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The server recovers; asking again has to try again.
	fail.Store(false)
	SetCoverArt("")
	SetCoverArt(url)
	waitForCover(t)
}

// The terminal can drop a placement without saying so — a repaint on pause
// did, and the cover never came back because we thought it was still placed.
// Placements are therefore re-issued after a while.
func TestPlacementsAreReissuedAfterAWhile(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	resetCover(t)
	out := capturePlacements(t)

	previous := placementRefresh
	placementRefresh = 10 * time.Millisecond
	t.Cleanup(func() { placementRefresh = previous })

	SetCoverArt(serveCover(t, 300, 300))
	waitForCover(t)
	RenderCover(10, 26)

	out.Reset()
	RenderCover(10, 26)
	if immediate := out.String(); immediate != "" {
		t.Errorf("re-placed straight away: %q", immediate)
	}

	time.Sleep(20 * time.Millisecond)
	out.Reset()
	RenderCover(10, 26)
	if !strings.Contains(out.String(), "a=p") {
		t.Error("the placement was never re-issued")
	}
}

// The now-playing screen: art, details, a progress bar and what plays next.
func TestCoverModeNowPlayingScreen(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Setenv("GRBFY_COVER_CELL_ASPECT", "2")
	coverAspectFixed, coverCellAspect = true, 2
	t.Cleanup(func() { coverAspectFixed = false })
	resetCover(t)
	t.Cleanup(SetGraphicsOutput(io.Discard))

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	defer WithPanelWidth(74)()
	d := &coverDriver{ctx: VisCoverContext{
		TrackTitle: "Kerala", TrackArtist: "Bonobo", AlbumLine: "Migration · 2017",
		PositionSecs: 72, DurationSecs: 225, NextLine: "Outlier — Bonobo",
	}}
	out := d.Render(&Visualizer{Rows: 12})
	plain := ansiRe.ReplaceAllString(out, "")

	for _, want := range []string{"Kerala", "Bonobo", "Migration · 2017", "1:12", "3:45", "next · Outlier — Bonobo"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q is missing from the screen", want)
		}
	}
	if !strings.Contains(plain, coverBarFill) || !strings.Contains(plain, coverBarEmpty) {
		t.Error("the progress bar is missing its filled or empty part")
	}
	// A third of the way in: the filled part must be shorter than the rest.
	filled := strings.Count(plain, coverBarFill)
	empty := strings.Count(plain, coverBarEmpty)
	if filled == 0 || filled >= empty {
		t.Errorf("bar shows %d filled vs %d empty for 72s of 225s", filled, empty)
	}
	if lines := strings.Split(out, "\n"); len(lines) != 12 {
		t.Errorf("screen has %d lines, want 12", len(lines))
	}
	for i, line := range strings.Split(out, "\n") {
		if n := cellWidth(ansiRe.ReplaceAllString(line, "")); n > PanelWidth {
			t.Errorf("line %d is %d cells wide, want <= %d", i, n, PanelWidth)
		}
	}
}

// A live stream has no duration, so a bar would be a lie.
func TestCoverModeLiveStreamHasNoBar(t *testing.T) {
	t.Setenv("GRBFY_CLOCK_GRAPHICS", "1")
	t.Setenv("GRBFY_COVER_CELL_ASPECT", "2")
	coverAspectFixed, coverCellAspect = true, 2
	t.Cleanup(func() { coverAspectFixed = false })
	resetCover(t)
	t.Cleanup(SetGraphicsOutput(io.Discard))

	SetCoverArt(serveCover(t, 640, 640))
	waitForCover(t)

	defer WithPanelWidth(74)()
	d := &coverDriver{ctx: VisCoverContext{TrackTitle: "Radio", DurationSecs: 0, PositionSecs: 900}}
	plain := ansiRe.ReplaceAllString(d.Render(&Visualizer{Rows: 12}), "")
	if strings.Contains(plain, coverBarFill) {
		t.Error("drew a progress bar for a stream with no duration")
	}
}

// The accent is the cover's own colour, not an average of everything in it.
func TestCoverAccentPicksTheDominantColour(t *testing.T) {
	// Three quarters dark grey, one quarter strong teal: the teal wins.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			c := color.RGBA{0x20, 0x20, 0x20, 0xFF}
			if x >= 50 && y >= 50 {
				c = color.RGBA{0x14, 0x8F, 0x96, 0xFF}
			}
			img.Set(x, y, c)
		}
	}
	got, ok := coverAccent(img)
	if !ok {
		t.Fatal("no accent found in a cover with an obvious colour")
	}
	if got.B < got.R || got.G < got.R {
		t.Errorf("accent %v is not the teal in the artwork", got)
	}

	// A greyscale sleeve has no accent worth using.
	grey := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 40 {
			grey.Set(x, y, color.RGBA{0x80, 0x80, 0x80, 0xFF})
		}
	}
	if _, ok := coverAccent(grey); ok {
		t.Error("found an accent in a greyscale cover")
	}
}
