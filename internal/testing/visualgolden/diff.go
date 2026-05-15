// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

//go:build !windows

package visualgolden

import (
	"image"
	"image/color"
	"image/draw"
)

// Diff computes pixel-level difference between two images using YIQ
// luminance distance (mapbox/pixelmatch algorithm port).
//
// pixelSensitivity (0..1): smaller = stricter (more pixels flagged).
// 0.1 is conservative; 0.01 is very strict. Returns:
//   - mismatchedPixels: count of pixels that differ beyond sensitivity
//   - diffImage:        RGBA overlay with diffs highlighted red; nil if
//                       images differ in dimension (caller handles).
//
// R5 spec § 2.4 distinct from Compare's maxMismatchFraction to avoid
// reversed-semantic confusion.
func Diff(a, b image.Image, pixelSensitivity float64) (int, image.Image) {
	if a.Bounds() != b.Bounds() {
		return -1, nil
	}
	bounds := a.Bounds()
	out := image.NewRGBA(bounds)
	// Background: grayscale 50% of a (so diffs stand out).
	draw.Draw(out, bounds, a, bounds.Min, draw.Src)
	desaturate(out, bounds)

	threshold := pixelSensitivity * maxYIQDistance
	mismatched := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			da := pixelYIQ(a.At(x, y))
			db := pixelYIQ(b.At(x, y))
			if yiqDistance(da, db) > threshold {
				mismatched++
				out.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}
	}
	return mismatched, out
}

// yiqValues are YIQ luminance + chrominance per pixel.
type yiqValues struct{ y, i, q float64 }

// pixelYIQ converts an RGBA color to YIQ. RGB in [0, 65535].
func pixelYIQ(c color.Color) yiqValues {
	r, g, b, _ := c.RGBA()
	// Normalize to [0, 255].
	rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)
	return yiqValues{
		y: 0.299*rf + 0.587*gf + 0.114*bf,
		i: 0.596*rf - 0.275*gf - 0.321*bf,
		q: 0.212*rf - 0.523*gf + 0.311*bf,
	}
}

func yiqDistance(a, b yiqValues) float64 {
	dy, di, dq := a.y-b.y, a.i-b.i, a.q-b.q
	// Weighted: luminance dominates per pixelmatch reference.
	return dy*dy*0.5053 + di*di*0.299 + dq*dq*0.1957
}

// maxYIQDistance is the empirical maximum distance for fully different
// black-vs-white pixels per the YIQ weights above (~35215.0). Used to
// normalize pixelSensitivity 0..1 into the unbounded distance scale.
const maxYIQDistance = 35215.0

// desaturate converts an RGBA image to grayscale in-place.
func desaturate(img *image.RGBA, bounds image.Rectangle) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.RGBAAt(x, y)
			gray := uint8(0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B))
			// Background grayscale at 50% intensity (mute the underlay).
			half := gray / 2
			img.SetRGBA(x, y, color.RGBA{R: half, G: half, B: half, A: 255})
		}
	}
}
