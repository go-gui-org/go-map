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
	const z = 10.0
	mppEq := metersPerPixel(0, z)
	mpp60 := metersPerPixel(60, z)
	ratio := mpp60 / mppEq
	want := math.Cos(60 * math.Pi / 180) // 0.5
	if math.Abs(ratio-want) > 1e-9 {
		t.Errorf("ratio mpp(60)/mpp(0) = %g, want %g", ratio, want)
	}
}

// zoomLabel drops the decimal for integer-valued zooms so the HUD
// and a11y string read "z12" at wheel-nav rest states, and keeps one
// digit for FitBounds / SetZoom-produced fractional zooms.
func TestZoomLabel_IntegerVsFraction(t *testing.T) {
	cases := []struct {
		z    float64
		want string
	}{
		{0, "0"},
		{12, "12"},
		{22, "22"},
		{12.4, "12.4"},
		{0.5, "0.5"},
		{math.NaN(), "0"},
		{math.Inf(1), "0"},
		{math.Inf(-1), "0"},
		{-0.5, "0"}, // negative must not leak through to uint64(z)
		{-3, "0"},
	}
	for _, c := range cases {
		if got := zoomLabel(c.z); got != c.want {
			t.Errorf("zoomLabel(%v) = %q, want %q", c.z, got, c.want)
		}
	}
}

// metersPerPixel at a fractional zoom must land on the continuous
// interpolation between the bracketing integer zooms — a refactor
// that accidentally floored z would pass TestMetersPerPixel_LatitudeScaling
// (integer-only) while breaking the scalebar at fractional state.
func TestMetersPerPixel_FractionalContinuity(t *testing.T) {
	mpp10 := metersPerPixel(0, 10)
	mpp11 := metersPerPixel(0, 11)
	mpp10_5 := metersPerPixel(0, 10.5)
	// Going up one zoom halves mpp; half-way should equal mpp10/sqrt(2).
	want := mpp10 / math.Sqrt2
	if rel := math.Abs(mpp10_5-want) / want; rel > 1e-9 {
		t.Errorf("mpp(z=10.5) = %g, want %g (rel err %g)",
			mpp10_5, want, rel)
	}
	// Sanity: mpp10.5 must sit strictly between mpp11 and mpp10.
	if !(mpp11 < mpp10_5 && mpp10_5 < mpp10) {
		t.Errorf("mpp bracketing failed: mpp11=%g mpp10.5=%g mpp10=%g",
			mpp11, mpp10_5, mpp10)
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
