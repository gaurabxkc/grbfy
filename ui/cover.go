package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"github.com/bjarneo/cliamp/applog"
)

// Album art as a real image, drawn the same way as the pomodoro clock: the
// picture is transmitted to the terminal out of band, and the view prints
// Unicode placeholder cells that the terminal composites it behind. See
// kittyclock.go for why that indirection is the only thing Bubbletea's cell
// renderer lets through.
//
// A character grid cannot show a cover any other way: two colours per cell
// (the halves of a block character) top out around 60x40 pixels for a
// fullscreen cover, which reads as a mosaic rather than artwork.

const (
	// The cover can be on screen twice at once — in the settings pane and in
	// the visualizer — at different sizes. They need an image id each: a
	// placeholder names its image by id, so sharing one would make the two
	// placements fight, which shows as the picture flickering between sizes.
	// Both sit above the clock's glyph ids so nothing collides.
	coverImageID      = 7400
	coverBlockImageID = 7401

	// coverMaxPixels caps the transmitted image. Terminals scale it down to
	// the placement box, and a 640px cover is already more than a panel can
	// show, so anything larger only costs encode time and bandwidth.
	coverMaxPixels = 640

	coverFetchTimeout = 10 * time.Second
)

// coverCellAspect is the terminal's cell height divided by its width, used to
// keep a cover from coming out stretched. Deliberately separate from the
// clock's cell_aspect: that one is tuned by eye for the size of the digits,
// while a photograph has to keep its real proportions. Most terminals are
// close to 2; GRBFY_COVER_CELL_ASPECT overrides it.
var coverCellAspect = 2.0

// coverAspectFixed marks the ratio as settled by the environment, so a later
// measurement does not overrule what the user asked for.
var coverAspectFixed bool

func init() {
	if v, err := strconv.ParseFloat(os.Getenv("GRBFY_COVER_CELL_ASPECT"), 64); err == nil && v >= 1 && v <= 5 {
		coverCellAspect, coverAspectFixed = v, true
	}
}

// refreshCoverCellAspect measures the terminal's real cell shape. Guessing it
// is what leaves a cover letterboxed: the terminal keeps the picture's own
// proportions inside the cell box it is given, so a box that is not the same
// shape as the image shows margins around it.
func refreshCoverCellAspect() {
	if coverAspectFixed {
		return
	}
	measured := terminalCellAspect()
	if measured <= 0 {
		return
	}
	// The reported pixel size rounds differently from frame to frame, so only
	// a real change is worth acting on — and worth a log line, which used to
	// be written on nearly every render.
	if math.Abs(measured-coverCellAspect) < 0.02 {
		return
	}
	applog.Info("cover: terminal cell aspect %.2f (was %.2f)", measured, coverCellAspect)
	coverCellAspect = measured
}

// coverState is one image id's contents.
type coverState struct {
	url        string
	cols, rows int
}

var (
	coverMu sync.Mutex
	// coverURL is what the model last asked for; coverImg is the decoded
	// picture for it, nil until the fetch lands. coverSentURL is what the
	// terminal currently holds under coverImageID.
	coverURL    string
	coverImg    image.Image
	coverTint   color.RGBA
	coverTinted bool
	// coverFetching is the URL a fetch is in flight for, so a second request
	// for it does not start another.
	coverFetching string
	// coverCache holds the last few decoded covers. Playback pausing, or any
	// other blip that briefly leaves no playing track, clears the current
	// artwork; without a cache the same URL coming back would be treated as
	// already fetched and never drawn again.
	coverCache     = map[string]image.Image{}
	coverCacheKeys []string
	// coverSent is what each image id currently holds: the artwork's URL and
	// the cell box it was scaled for. A different box needs a fresh scale, or
	// the terminal fits the picture itself and rounds each row as it goes.
	coverSent = map[int]coverState{}

	coverClient = &http.Client{Timeout: coverFetchTimeout}
)

// SetCoverArt records the artwork for the playing track and starts fetching it
// when it is new. Called from the tick context, so it must not block.
func SetCoverArt(url string) {
	coverMu.Lock()
	defer coverMu.Unlock()
	if url == coverURL {
		return
	}
	coverURL = url
	coverImg = coverCache[url]
	coverTint, coverTinted = coverAccent(coverImg)
	if url == "" || coverImg != nil || coverFetching == url {
		return
	}
	coverFetching = url
	go fetchCover(url)
}

// cacheCoverLocked remembers a decoded cover, keeping the last few so going
// back to a recent track redraws at once. coverMu must be held.
func cacheCoverLocked(url string, img image.Image) {
	const keep = 4
	if _, seen := coverCache[url]; !seen {
		coverCacheKeys = append(coverCacheKeys, url)
	}
	coverCache[url] = img
	for len(coverCacheKeys) > keep {
		delete(coverCache, coverCacheKeys[0])
		coverCacheKeys = coverCacheKeys[1:]
	}
}

func fetchCover(url string) {
	// Whatever happens, stop claiming a fetch is in flight, or a failure
	// would block every later attempt at the same artwork.
	done := true
	defer func() {
		if !done {
			return
		}
		coverMu.Lock()
		if coverFetching == url {
			coverFetching = ""
		}
		coverMu.Unlock()
	}()

	resp, err := coverClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	// Covers are a few hundred KB; the cap keeps a wrong URL from filling
	// memory.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	img = shrinkCover(img)

	done = false // the locked section below clears the marker itself
	coverMu.Lock()
	defer coverMu.Unlock()
	cacheCoverLocked(url, img)
	if coverURL == url {
		coverTint, coverTinted = coverAccent(img)
	}
	if coverFetching == url {
		coverFetching = ""
	}
	// The track may have changed while this was in flight.
	if coverURL == url {
		coverImg = img
	}
}

// shrinkCover scales an oversized cover down, keeping its proportions.
func shrinkCover(src image.Image) image.Image {
	b := src.Bounds()
	longest := max(b.Dx(), b.Dy())
	if longest <= coverMaxPixels {
		return src
	}
	scale := float64(coverMaxPixels) / float64(longest)
	dst := image.NewRGBA(image.Rect(0, 0, int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// transmitCover sends the current cover under coverImageID, replacing whatever
// was there. Deleting first also drops the old image's placements, so the
// caller must place it again afterwards (the clock learned this the hard way).
func transmitCover(w io.Writer, img image.Image, url string, id, cols, rows int) bool {
	img = fitToCellBox(img, cols, rows)

	var enc bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&enc, img); err != nil {
		return false
	}
	b64 := base64.StdEncoding.EncodeToString(enc.Bytes())

	var out bytes.Buffer
	fmt.Fprintf(&out, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
	const chunk = 4000
	for p := 0; p < len(b64); p += chunk {
		end := min(p+chunk, len(b64))
		more := 0
		if end < len(b64) {
			more = 1
		}
		if p == 0 {
			fmt.Fprintf(&out, "\x1b_Gf=100,a=t,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, b64[p:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, b64[p:end])
		}
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		return false
	}
	forgetPlacements(id)
	coverSent[id] = coverState{url: url, cols: cols, rows: rows}
	return true
}

// fitToCellBox scales the artwork to exactly the pixels the cell box covers,
// so the terminal has no fitting left to do. Leaving that to the terminal is
// what leaves a picture letterboxed inside its box, and rounds rows against a
// fractional scale, which shows as part of the image sitting a column off.
//
// The box is chosen to match the picture's proportions to within a cell, so
// this stretch is under half a cell and invisible; without the terminal's cell
// size there is nothing to scale to, and the image is sent as it is.
func fitToCellBox(src image.Image, cols, rows int) image.Image {
	cellW, cellH, ok := terminalCellPixels()
	if !ok || cols <= 0 || rows <= 0 {
		return src
	}
	w := int(float64(cols)*cellW + 0.5)
	h := int(float64(rows)*cellH + 0.5)
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return src
	}
	b := src.Bounds()
	if b.Dx() == w && b.Dy() == h {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// coverBox returns the cell box a square-ish cover should occupy inside the
// panel, keeping the picture's proportions: cells are about coverCellAspect
// times taller than they are wide, so a square needs that many more columns
// than rows.
func coverBox(img image.Image, rows, cols int) (boxRows, boxCols int) {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || rows <= 0 || cols <= 0 {
		return 0, 0
	}
	ratio := float64(b.Dx()) / float64(b.Dy()) * coverCellAspect
	boxRows = rows
	boxCols = int(float64(boxRows)*ratio + 0.5)
	if boxCols > cols {
		boxCols = cols
		boxRows = int(float64(boxCols)/ratio + 0.5)
	}
	boxRows = min(boxRows, len(rowColumnDiacritics))
	boxCols = min(boxCols, len(rowColumnDiacritics))
	return boxRows, boxCols
}

// coverRows renders the placeholder cells for the placed image, centred in the
// panel. Each cell names the image in its foreground colour and its own row and
// column through combining marks, exactly as the clock's glyphs do.
func coverRows(id, rows, cols, boxRows, boxCols int) []string {
	pad := strings.Repeat(" ", max(0, (cols-boxCols)/2))
	out := make([]string, 0, rows)
	for range max(0, (rows-boxRows)/2) {
		out = append(out, "")
	}
	for r := range boxRows {
		var sb strings.Builder
		sb.WriteString(pad)
		fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", (id>>16)&0xFF, (id>>8)&0xFF, id&0xFF)
		for c := range boxCols {
			sb.WriteRune(0x10EEEE)
			sb.WriteRune(rowColumnDiacritics[r])
			sb.WriteRune(rowColumnDiacritics[c])
		}
		sb.WriteString("\x1b[0m")
		out = append(out, sb.String())
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return out[:rows]
}

// CoverAccent is the playing artwork's accent colour, or false when there is
// no artwork or it has no colour worth using.
func CoverAccent() (color.RGBA, bool) {
	coverMu.Lock()
	defer coverMu.Unlock()
	return coverTint, coverTinted
}

// CoverRowsFor reports how many rows the artwork needs at the given width,
// keeping its proportions, or 0 when there is nothing to draw. Callers size a
// region with this before asking for the render.
func CoverRowsFor(cols int) int {
	if !ClockGraphicsAvailable() || cols < 2 {
		return 0
	}
	refreshCoverCellAspect()
	coverMu.Lock()
	img := coverImg
	coverMu.Unlock()
	if img == nil {
		return 0
	}
	rows, _ := coverBox(img, len(rowColumnDiacritics), cols)
	return rows
}

// RenderCoverBlock draws the artwork exactly rows tall, with no padding
// around it, and reports the width it came out. Callers that place the cover
// themselves — beside text, inside a column — use this; RenderCover centres
// it in a region instead.
func RenderCoverBlock(rows int) (lines []string, width int, ok bool) {
	if !ClockGraphicsAvailable() || rows < 2 {
		return nil, 0, false
	}
	refreshCoverCellAspect()

	coverMu.Lock()
	img, url, sent := coverImg, coverURL, coverSent[coverBlockImageID]
	coverMu.Unlock()
	if img == nil || url == "" {
		return nil, 0, false
	}

	// An unbounded width asks coverBox for the natural width at this height.
	boxRows, boxCols := coverBox(img, rows, len(rowColumnDiacritics))
	if boxRows < 2 || boxCols < 2 {
		return nil, 0, false
	}

	if (sent != coverState{url: url, cols: boxCols, rows: boxRows}) {
		coverMu.Lock()
		written := transmitCover(placementOut, img, url, coverBlockImageID, boxCols, boxRows)
		coverMu.Unlock()
		if !written {
			return nil, 0, false
		}
	}
	ensurePlacement(coverBlockImageID, boxCols, boxRows)

	return coverRows(coverBlockImageID, boxRows, boxCols, boxRows, boxCols), boxCols, true
}

// RenderCover draws the current album art, or reports false when there is
// nothing to draw: no artwork, not fetched yet, or a terminal without the
// graphics protocol. The caller then falls back to text.
func RenderCover(rows, cols int) (string, bool) {
	if !ClockGraphicsAvailable() || rows < 2 || cols < 2 {
		return "", false
	}

	coverMu.Lock()
	img, url := coverImg, coverURL
	sent := coverSent[coverImageID]
	coverMu.Unlock()
	if img == nil || url == "" {
		return "", false
	}

	boxRows, boxCols := coverBox(img, rows, cols)
	if boxRows < 2 || boxCols < 2 {
		return "", false
	}

	if (sent != coverState{url: url, cols: boxCols, rows: boxRows}) {
		coverMu.Lock()
		ok := transmitCover(placementOut, img, url, coverImageID, boxCols, boxRows)
		coverMu.Unlock()
		if !ok {
			return "", false
		}
	}
	ensurePlacement(coverImageID, boxCols, boxRows)

	return strings.Join(coverRows(coverImageID, rows, cols, boxRows, boxCols), "\n"), true
}
