package ui

import (
	"image"
	"image/color"
	"math"
)

// The cover's accent colour: what the artwork would be called if you had to
// name one colour in it. Used to tint the now-playing screen's progress bar,
// so the visualizer picks up the record's own palette rather than a fixed
// theme colour.
//
// Averaging the whole picture gives mud — opposite hues cancel — so colours
// are bucketed by hue and the heaviest bucket wins, weighted towards colours
// with some life in them. Near-black, near-white and grey pixels are skipped:
// they are the background of most covers and would swamp everything else.
const (
	accentBuckets   = 24
	accentMinSat    = 0.18
	accentMinLum    = 0.12
	accentMaxLum    = 0.92
	accentSampleDim = 64
)

// coverAccent picks the accent colour for an image, or reports false when the
// artwork has no colour worth using (a greyscale sleeve, say).
func coverAccent(img image.Image) (color.RGBA, bool) {
	if img == nil {
		return color.RGBA{}, false
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return color.RGBA{}, false
	}

	// Sample a grid rather than every pixel: a cover is 640px square and the
	// answer does not change for looking at all of it.
	stepX := max(1, b.Dx()/accentSampleDim)
	stepY := max(1, b.Dy()/accentSampleDim)

	var (
		weights [accentBuckets]float64
		sums    [accentBuckets][3]float64
	)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x8000 {
				continue
			}
			r, g, bl := float64(r16>>8)/255, float64(g16>>8)/255, float64(b16>>8)/255
			hue, sat, lum := hsl(r, g, bl)
			if sat < accentMinSat || lum < accentMinLum || lum > accentMaxLum {
				continue
			}
			// Saturation as weight: a vivid pixel says more about a cover's
			// colour than a washed-out one.
			w := sat * sat
			i := int(hue*accentBuckets) % accentBuckets
			weights[i] += w
			sums[i][0] += r * w
			sums[i][1] += g * w
			sums[i][2] += bl * w
		}
	}

	best := -1
	for i, w := range weights {
		if best < 0 || w > weights[best] {
			best = i
		}
	}
	if best < 0 || weights[best] == 0 {
		return color.RGBA{}, false
	}

	w := weights[best]
	r, g, bl := sums[best][0]/w, sums[best][1]/w, sums[best][2]/w
	// Lift very dark accents so the colour reads against a dark terminal.
	if _, _, lum := hsl(r, g, bl); lum < 0.35 {
		scale := 0.35 / math.Max(lum, 0.01)
		r, g, bl = math.Min(r*scale, 1), math.Min(g*scale, 1), math.Min(bl*scale, 1)
	}
	return color.RGBA{uint8(r*255 + 0.5), uint8(g*255 + 0.5), uint8(bl*255 + 0.5), 255}, true
}

// hsl returns hue (0..1), saturation (0..1) and lightness (0..1).
func hsl(r, g, b float64) (hue, sat, lum float64) {
	maxC, minC := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	lum = (maxC + minC) / 2
	if maxC == minC {
		return 0, 0, lum
	}
	d := maxC - minC
	if lum > 0.5 {
		sat = d / (2 - maxC - minC)
	} else {
		sat = d / (maxC + minC)
	}
	switch maxC {
	case r:
		hue = (g - b) / d
		if g < b {
			hue += 6
		}
	case g:
		hue = (b-r)/d + 2
	default:
		hue = (r-g)/d + 4
	}
	return hue / 6, sat, lum
}
