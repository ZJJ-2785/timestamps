package ring

import (
	"image/color"
	"math"
	"testing"
)

func TestColor(t *testing.T) {
	if c := Color(0); c != stops[0].c {
		t.Errorf("start %v", c)
	}
	if c := Color(1); c != stops[len(stops)-1].c {
		t.Errorf("end %v", c)
	}
	if c := Color(0.45); c != stops[1].c {
		t.Errorf("middle %v", c)
	}
	if c := Color(0.6); c.R < 242 || c.G > 197 || c.G < 146 {
		t.Errorf("between yellow and orange: %v", c)
	}
}

func TestDraw(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	const s = 32
	img := Draw(s, 0.5, red)
	// Right half (3 o'clock) is on the arc, left half (9 o'clock) only track.
	right, left := img.NRGBAAt(s-4, s/2), img.NRGBAAt(3, s/2)
	if right.R < 200 || right.A < 200 {
		t.Errorf("3 o'clock should be the arc: %v", right)
	}
	if left.R > 200 || left.A == 0 || left.A > 160 {
		t.Errorf("9 o'clock should be faint track: %v", left)
	}
	if c := img.NRGBAAt(s/2, s/2); c.A != 0 {
		t.Errorf("center should be empty: %v", c)
	}
	// The round ends stay within the arc: at 06:00 it ends at 3 o'clock.
	if c := Draw(s, 0.25, red).NRGBAAt(28, 17); c.R > 200 {
		t.Errorf("just past 3 o'clock at 06:00: %v", c)
	}
	// Late in the evening a gap stays open before 12 o'clock, at tray size too.
	for _, size := range []int{16, 32} {
		img := Draw(size, 279.0/288, red) // 23:15
		mid := float64(size)/2 - math.Max(2, float64(size)*0.17)/2 - float64(size)*0.03
		if c := img.NRGBAAt(size/2-1, int(float64(size)/2-mid)); c.R > 200 {
			t.Errorf("%d px at 23:15: no gap before 12 o'clock: %v", size, c)
		}
		if c := Draw(size, 287.0/288, red).NRGBAAt(size/2-1, int(float64(size)/2-mid)); c.R > 200 {
			t.Errorf("%d px at 23:55: no gap before 12 o'clock: %v", size, c)
		}
	}
	// Nothing filled at the very start of the day.
	empty := Draw(s, 0, red)
	if c := empty.NRGBAAt(s-4, s/2); c.R > 200 {
		t.Errorf("empty ring has arc: %v", c)
	}
}
