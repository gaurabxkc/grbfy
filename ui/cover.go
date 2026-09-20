package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
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
	// coverImageID sits above the clock's glyph ids so the two never collide.
	coverImageID = 7400

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

func init() {
	if v, err := strconv.ParseFloat(os.Getenv("GRBFY_COVER_CELL_ASPECT"), 64); err == nil && v >= 1 && v <= 5 {
		coverCellAspect = v
	}
}

var (
	coverMu sync.Mutex
	// coverURL is what the model last asked for; coverImg is the decoded
	// picture for it, nil until the fetch lands. coverSentURL is what the
	// terminal currently holds under coverImageID.
	coverURL     string
	coverImg     image.Image
	coverFetched string
	coverSentURL string

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
	coverImg = nil
	if url == "" || coverFetched == url {
		return
	}
	coverFetched = url
	go fetchCover(url)
}

func fetchCover(url string) {
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

	coverMu.Lock()
	defer coverMu.Unlock()
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
func transmitCover(w io.Writer, img image.Image, url string) bool {
	var enc bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&enc, img); err != nil {
		return false
	}
	b64 := base64.StdEncoding.EncodeToString(enc.Bytes())

	var out bytes.Buffer
	fmt.Fprintf(&out, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", coverImageID)
	const chunk = 4000
	for p := 0; p < len(b64); p += chunk {
		end := min(p+chunk, len(b64))
		more := 0
		if end < len(b64) {
			more = 1
		}
		if p == 0 {
			fmt.Fprintf(&out, "\x1b_Gf=100,a=t,t=d,i=%d,q=2,m=%d;%s\x1b\\", coverImageID, more, b64[p:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, b64[p:end])
		}
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		return false
	}
	forgetPlacements()
	coverSentURL = url
	return true
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
func coverRows(rows, cols, boxRows, boxCols int) []string {
	pad := strings.Repeat(" ", max(0, (cols-boxCols)/2))
	out := make([]string, 0, rows)
	for range max(0, (rows-boxRows)/2) {
		out = append(out, "")
	}
	for r := range boxRows {
		var sb strings.Builder
		sb.WriteString(pad)
		fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", (coverImageID>>16)&0xFF, (coverImageID>>8)&0xFF, coverImageID&0xFF)
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

// CoverRowsFor reports how many rows the artwork needs at the given width,
// keeping its proportions, or 0 when there is nothing to draw. Callers size a
// region with this before asking for the render.
func CoverRowsFor(cols int) int {
	if !ClockGraphicsAvailable() || cols < 2 {
		return 0
	}
	coverMu.Lock()
	img := coverImg
	coverMu.Unlock()
	if img == nil {
		return 0
	}
	rows, _ := coverBox(img, len(rowColumnDiacritics), cols)
	return rows
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
	sent := coverSentURL
	coverMu.Unlock()
	if img == nil || url == "" {
		return "", false
	}

	boxRows, boxCols := coverBox(img, rows, cols)
	if boxRows < 2 || boxCols < 2 {
		return "", false
	}

	if sent != url {
		coverMu.Lock()
		ok := transmitCover(placementOut, img, url)
		coverMu.Unlock()
		if !ok {
			return "", false
		}
	}
	ensurePlacement(coverImageID, boxCols, boxRows)

	return strings.Join(coverRows(rows, cols, boxRows, boxCols), "\n"), true
}
