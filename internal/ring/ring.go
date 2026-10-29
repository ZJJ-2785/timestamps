// Package ring draws the tray icon: a ring that fills clockwise from
// 12 o'clock once a day, its color moving from green through yellow and
// orange to red as the day goes by.
package ring

import (
	"image"
	"image/color"
	"math"
)

// Stops of the color along the day (fraction 0..1).
var stops = []struct {
	at float64
	c  color.NRGBA
}{
	{0, color.NRGBA{63, 178, 127, 255}},    // green
	{0.45, color.NRGBA{242, 197, 61, 255}}, // yellow
	{0.75, color.NRGBA{245, 146, 47, 255}}, // orange
	{1, color.NRGBA{229, 72, 77, 255}},     // red
}

// Gray is the ring's color while tracking is paused.
var Gray = color.NRGBA{139, 147, 167, 255}

// Color returns the ring color for a fraction of the day.
func Color(frac float64) color.NRGBA {
	frac = math.Min(math.Max(frac, 0), 1)
	for i := 1; i < len(stops); i++ {
		if frac <= stops[i].at {
			a, b := stops[i-1], stops[i]
			t := (frac - a.at) / (b.at - a.at)
			mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
			return color.NRGBA{mix(a.c.R, b.c.R), mix(a.c.G, b.c.G), mix(a.c.B, b.c.B), 255}
		}
	}
	return stops[len(stops)-1].c
}

// gapMin is the least gap, in pixels along the middle of the band, that the
// arc leaves before 12 o'clock for a band width pixels wide.
func gapMin(width float64) float64 { return math.Max(1.5, width/2) }

// Draw returns a size×size ring filled up to frac (0..1) in c, with round
// ends, over a faint track that shows on light and dark taskbars.
func Draw(size int, frac float64, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	width := math.Max(2, s*0.17)
	mid := s/2 - width/2 - s*0.03 // radius of the middle of the band
	cx, cy := s/2, s/2
	end := math.Min(math.Max(frac, 0), 1) * 2 * math.Pi
	// The round ends stay within the arc, and the arc stops short of a full
	// circle: otherwise the ends close the gap the last hours leave (at tray
	// size from about 22:20) and the day reads as over.
	end = math.Min(end, 2*math.Pi-gapMin(width)/mid)
	a0 := width / 2 / mid // centers of the round ends, as angles
	a1 := math.Max(a0, end-a0)
	sx, sy := cx+mid*math.Sin(a0), cy-mid*math.Cos(a0)
	ex, ey := cx+mid*math.Sin(a1), cy-mid*math.Cos(a1)
	track := color.NRGBA{128, 134, 148, 110}
	const n = 4 // samples per side
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var arc, trk float64
			for j := 0; j < n; j++ {
				for i := 0; i < n; i++ {
					x := float64(px) + (float64(i)+0.5)/n
					y := float64(py) + (float64(j)+0.5)/n
					dx, dy := x-cx, y-cy
					inBand := math.Abs(math.Hypot(dx, dy)-mid) <= width/2
					a := math.Atan2(dx, -dy) // clockwise from 12 o'clock
					if a < 0 {
						a += 2 * math.Pi
					}
					onArc := frac > 0 && ((inBand && a >= a0 && a <= a1) || math.Hypot(x-sx, y-sy) <= width/2 || math.Hypot(x-ex, y-ey) <= width/2)
					switch {
					case onArc:
						arc++
					case inBand:
						trk++
					}
				}
			}
			arc /= n * n
			trk /= n * n
			ta := trk * float64(track.A) / 255
			alpha := arc + ta
			if alpha <= 0 {
				continue
			}
			ch := func(a, t uint8) uint8 { return uint8(math.Round((float64(a)*arc + float64(t)*ta) / alpha)) }
			img.SetNRGBA(px, py, color.NRGBA{ch(c.R, track.R), ch(c.G, track.G), ch(c.B, track.B), uint8(math.Round(math.Min(alpha, 1) * 255))})
		}
	}
	return img
}
