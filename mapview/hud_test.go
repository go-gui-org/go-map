package mapview

import (
	"math"
	"testing"
)

// TestNiceRound: scale-bar lengths must come from the {1,2,5}×10ⁿ
// sequence and never exceed the input ceiling.
func TestNiceRound(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0.9, 0.5},
		{1.0, 1},
		{1.5, 1},
		{2.0, 2},
		{4.9, 2},
		{5.0, 5},
		{9.0, 5},
		{10, 10},
		{47, 20},
		{99, 50},
		{100, 100},
		{4321, 2000},
		{50000, 50000},
	}
	for _, c := range cases {
		got := niceRound(c.in)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("niceRound(%g) = %g, want %g", c.in, got, c.want)
		}
	}
}

// TestNiceRoundZero: non-positive input must return zero, not NaN
// from log10 of a non-positive value.
func TestNiceRoundZero(t *testing.T) {
	for _, v := range []float64{0, -1, -0.5} {
		if got := niceRound(v); got != 0 {
			t.Errorf("niceRound(%g) = %g, want 0", v, got)
		}
	}
}

// NaN must short-circuit to 0; otherwise log10(NaN) propagates and
// the scale-bar rendering path draws NaN-sized geometry.
func TestNiceRound_NaNReturnsZero(t *testing.T) {
	if got := niceRound(math.NaN()); got != 0 {
		t.Errorf("niceRound(NaN) = %g, want 0", got)
	}
}

func TestNiceRound_InfReturnsZero(t *testing.T) {
	for _, v := range []float64{math.Inf(1), math.Inf(-1)} {
		if got := niceRound(v); got != 0 {
			t.Errorf("niceRound(%g) = %g, want 0", v, got)
		}
	}
}

// math.Pow(10, exp) returns +Inf for very large maxValue. The guard
// must catch that and return 0 rather than divide Inf/Inf into NaN.
func TestNiceRound_ExtremeMagnitudeNoPanic(t *testing.T) {
	for _, v := range []float64{1e308, math.MaxFloat64} {
		got := niceRound(v)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("niceRound(%g) = %g, want finite", v, got)
		}
	}
}

// TestMetricBar_FitsInRange: chosen length must never exceed maxPx
// and the label must round-trip through the formatter.
func TestMetricBar_FitsInRange(t *testing.T) {
	const maxPx float32 = 110
	cases := []struct {
		mpp float64
	}{
		{0.5}, {1}, {10}, {76.4}, {1000}, {10000}, {153543}}
	for _, c := range cases {
		label, px := metricBar(c.mpp, maxPx)
		if label == "" {
			t.Errorf("mpp=%g produced empty label", c.mpp)
		}
		if px > maxPx+0.01 {
			t.Errorf("mpp=%g px=%g exceeds maxPx=%g", c.mpp, px, maxPx)
		}
		if px <= 0 {
			t.Errorf("mpp=%g produced non-positive px=%g", c.mpp, px)
		}
	}
}

// TestImperialBar_FeetToMilesCrossover: just below the mile boundary
// the label must use feet; well above it must use miles.
func TestImperialBar_FeetToMilesCrossover(t *testing.T) {
	const maxPx float32 = 110
	feetLabel, _ := imperialBar(1, maxPx) // 1 m/px → ~360 ft fits
	if feetLabel == "" || feetLabel[len(feetLabel)-2:] != "ft" {
		t.Errorf("expected feet label at 1 m/px, got %q", feetLabel)
	}
	miLabel, _ := imperialBar(100, maxPx)
	if miLabel == "" || miLabel[len(miLabel)-2:] != "mi" {
		t.Errorf("expected miles label at 100 m/px, got %q", miLabel)
	}
}

// TestMetersPerPixel_LatitudeScaling: at a given zoom, distance per
// pixel must shrink by cos(lat) as latitude increases.
func TestMetersPerPixel_LatitudeScaling(t *testing.T) {
	const z uint32 = 10
	mppEq := metersPerPixel(0, z)
	mpp60 := metersPerPixel(60, z)
	ratio := mpp60 / mppEq
	want := math.Cos(60 * math.Pi / 180) // 0.5
	if math.Abs(ratio-want) > 1e-9 {
		t.Errorf("ratio mpp(60)/mpp(0) = %g, want %g", ratio, want)
	}
}

// TestHomeButtonHit: rect helper must agree with hit-test for
// in-bounds and out-of-bounds points.
func TestHomeButtonHit(t *testing.T) {
	const canvasW float32 = 800
	x, y, w, h := homeButtonRect(canvasW)
	cases := []struct {
		mx, my float32
		want   bool
	}{
		{x + w/2, y + h/2, true},
		{x - 1, y + h/2, false},
		{x + w + 1, y + h/2, false},
		{x + w/2, y - 1, false},
		{x + w/2, y + h + 1, false},
		{x, y, true}, // top-left inclusive
		{x + w - 0.01, y + h - 0.01, true},
	}
	for _, c := range cases {
		got := homeButtonHit(canvasW, c.mx, c.my)
		if got != c.want {
			t.Errorf("hit(%g,%g) = %v, want %v", c.mx, c.my, got, c.want)
		}
	}
}
