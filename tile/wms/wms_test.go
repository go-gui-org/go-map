package wms

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-gui-org/go-map/tile"
)

// pngFixture passes tile.IsPNG; test servers return it to satisfy the
// body validator without decoding real image bytes.
var pngFixture = []byte("\x89PNG\r\n\x1a\nfake-payload")

// jpegFixture passes tile.IsJPEG for Format=image/jpeg tests.
var jpegFixture = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

func validCfg() Cfg {
	return Cfg{
		Endpoint:    "https://ows.example.com/wms",
		Layers:      []string{"roads"},
		Attribution: "© Example",
		MaxZoom:     18,
	}
}

func TestNew_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Cfg)
	}{
		{"no_endpoint", func(c *Cfg) { c.Endpoint = "" }},
		{"no_layers", func(c *Cfg) { c.Layers = nil }},
		{"no_attribution", func(c *Cfg) { c.Attribution = "" }},
		{"no_maxzoom", func(c *Cfg) { c.MaxZoom = 0 }},
		{"empty_layer_name", func(c *Cfg) { c.Layers = []string{"a", ""} }},
		{"unsupported_format", func(c *Cfg) { c.Format = "image/webp" }},
	}
	for _, tc := range cases {
		cfg := validCfg()
		tc.mut(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: err = nil, want non-nil", tc.name)
		}
	}
}

func TestNew_DefaultsAndURLShape(t *testing.T) {
	src, err := New(Cfg{
		Endpoint:    "https://ows.example.com/wms",
		Layers:      []string{"roads", "labels"},
		Attribution: "© Example",
		MaxZoom:     18,
		Transparent: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	checks := map[string]string{
		"service":     "WMS",
		"request":     "GetMap",
		"version":     "1.3.0",
		"layers":      "roads,labels",
		"styles":      ",",
		"crs":         "EPSG:3857",
		"width":       "256",
		"height":      "256",
		"format":      "image/png",
		"transparent": "TRUE",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("q[%s] = %q, want %q", k, got, want)
		}
	}
	if q.Get("srs") != "" {
		t.Errorf("srs present (WMS 1.1.1 param), want crs only")
	}
}

func TestNew_TransparentDefaultFalse(t *testing.T) {
	src, _ := New(validCfg())
	u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
	q, _ := url.ParseQuery(strings.SplitN(u, "?", 2)[1])
	if got := q.Get("transparent"); got != "FALSE" {
		t.Errorf("transparent = %q, want FALSE", got)
	}
}

func TestBBoxFor_WorldTile(t *testing.T) {
	// Round-trip each emitted value and check the server-side parse
	// reproduces the tile corner to <1 mm — guards BBOX precision.
	got := bboxFor(tile.Coord{Z: 0, X: 0, Y: 0})
	parts := strings.Split(got, ",")
	if len(parts) != 4 {
		t.Fatalf("bbox parts = %d, want 4", len(parts))
	}
	exp := []float64{-mercatorR, -mercatorR, mercatorR, mercatorR}
	for i, v := range parts {
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatalf("part[%d] parse: %v", i, err)
		}
		if diff := parsed - exp[i]; diff < -0.001 || diff > 0.001 {
			t.Errorf("part[%d] = %g, want %g (diff %g)", i, parsed, exp[i], diff)
		}
	}
}

func TestBBoxFor_TopRightQuadrant(t *testing.T) {
	// Z=1 splits the world in 4. Tile (1,0) is the top-right quadrant:
	// x in [0, R], y in [0, R] (northern hemisphere east half).
	got := bboxFor(tile.Coord{Z: 1, X: 1, Y: 0})
	parts := strings.Split(got, ",")
	exp := []float64{0, 0, mercatorR, mercatorR}
	for i, v := range parts {
		parsed, _ := strconv.ParseFloat(v, 64)
		if diff := parsed - exp[i]; diff < -0.001 || diff > 0.001 {
			t.Errorf("part[%d] = %g, want %g", i, parsed, exp[i])
		}
	}
}

func TestNew_StylesPadsToLayerCount(t *testing.T) {
	// Two layers, one style. STYLES must have two slots: the first
	// with the explicit value, the second empty.
	src, _ := New(Cfg{
		Endpoint:    "https://ows.example.com/wms",
		Layers:      []string{"roads", "labels"},
		Styles:      []string{"night"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
	q, _ := url.ParseQuery(strings.SplitN(u, "?", 2)[1])
	if got := q.Get("styles"); got != "night," {
		t.Errorf("styles = %q, want %q", got, "night,")
	}
}

func TestNew_EndpointWithExistingQuery(t *testing.T) {
	// MapServer-style endpoint with a map= parameter already present
	// must concatenate with "&", not "?".
	src, _ := New(Cfg{
		Endpoint:    "https://example.com/cgi?map=foo.map",
		Layers:      []string{"roads"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
	if !strings.Contains(u, "map=foo.map&service=WMS") {
		t.Errorf("URL does not preserve existing query: %s", u)
	}
	if strings.Count(u, "?") != 1 {
		t.Errorf("URL has multiple %q: %s", "?", u)
	}
}

func TestNew_URLEncodesLayerNames(t *testing.T) {
	// Namespaced layer names contain ":" which must be URL-encoded to
	// survive proxies that reject reserved chars in query values.
	src, _ := New(Cfg{
		Endpoint:    "https://example.com/wms",
		Layers:      []string{"ws:my layer"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
	if !strings.Contains(u, "layers=ws%3Amy+layer") {
		t.Errorf("URL did not encode layer name: %s", u)
	}
}

func TestURL_InvalidCoord(t *testing.T) {
	src, _ := New(validCfg())
	if u := src.URL(tile.Coord{Z: 2, X: 4, Y: 0}); u != "" {
		t.Errorf("URL of invalid coord = %q, want \"\"", u)
	}
}

func TestFetch_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	wantUA := "my-app/1.0 (https://example.com)"
	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"roads"},
		Attribution: "© Example",
		MaxZoom:     18,
		UserAgent:   wantUA,
	})
	if _, err := src.Fetch(context.Background(),
		tile.Coord{Z: 0, X: 0, Y: 0}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotUA != wantUA {
		t.Errorf("UA = %q, want %q", gotUA, wantUA)
	}
}

func TestFetch_SanitizesUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"roads"},
		Attribution: "© Example",
		MaxZoom:     18,
		UserAgent:   "evil\r\nX-Bad: yes",
	})
	_, _ = src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if strings.ContainsAny(gotUA, "\r\n") {
		t.Errorf("UA contains CRLF: %q", gotUA)
	}
}

func TestFetch_RejectsServiceExceptionXML(t *testing.T) {
	// WMS servers return 200 OK with ServiceException XML on errors.
	// The body validator must reject this so go-gui never caches the
	// XML as an "image."
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w,
				`<?xml version="1.0"?><ServiceExceptionReport/>`)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	_, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want rejection")
	}
	if !strings.Contains(err.Error(), "not a image/png image") {
		t.Errorf("err = %v, want to mention format mismatch", err)
	}
}

func TestFetch_RejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	_, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want rejection")
	}
}

func TestFetch_AcceptsJPEGWhenFormatJPEG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(jpegFixture)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
		Format:      "image/jpeg",
	})
	body, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !tile.IsJPEG(body) {
		t.Error("body is not JPEG")
	}
}

func TestFetch_RejectsPNGWhenFormatJPEG(t *testing.T) {
	// Format-driven validation: if the server returns PNG bytes when
	// image/jpeg was requested, the body must be rejected.
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
		Format:      "image/jpeg",
	})
	_, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want rejection of PNG body when Format=image/jpeg")
	}
}

func TestFetch_404IsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	_, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if !errors.Is(err, tile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFetch_500WrapsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	_, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want to mention 500", err)
	}
}

func TestHTTPFetcher_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
		UserAgent:   "ua/1",
	})
	f, ok := src.(tile.HTTPFetcher)
	if !ok {
		t.Fatal("source does not implement tile.HTTPFetcher")
	}
	resp, err := f.HTTPFetcher()(context.Background(), srv.URL+"/irrelevant")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	_ = resp.Body.Close()
	if gotUA != "ua/1" {
		t.Errorf("UA = %q, want ua/1", gotUA)
	}
}

func TestHTTPFetcher_PassesThroughNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	defer srv.Close()

	src, _ := New(validCfg())
	f := src.(tile.HTTPFetcher).HTTPFetcher()
	resp, err := f(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("err = %v, want nil for non-200 passthrough", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// Cancelled context must surface from Fetch promptly so a caller that
// drops a tile request does not leak a goroutine blocked in HTTP.
// Covers the s.do plumbing shared by Fetch and HTTPFetcher.
func TestWMS_Fetch_PropagatesContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call
	_, err := src.Fetch(ctx, tile.Coord{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want context cancellation")
	}
	if !strings.Contains(err.Error(), "canceled") &&
		!strings.Contains(err.Error(), "context") {
		t.Errorf("err = %v, want context-cancellation", err)
	}
}

// LimitReader must cap response-body size; a misconfigured or hostile
// WMS server returning GBs must not OOM the caller.
func TestWMS_Fetch_LimitsResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
			_, _ = w.Write(make([]byte, maxBodyBytes+(1<<20)))
		}))
	defer srv.Close()

	src, _ := New(Cfg{
		Endpoint:    srv.URL,
		Layers:      []string{"x"},
		Attribution: "© Example",
		MaxZoom:     18,
	})
	body, err := src.Fetch(context.Background(), tile.Coord{Z: 0, X: 0, Y: 0})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if int64(len(body)) > maxBodyBytes {
		t.Errorf("body len = %d, exceeds cap %d", len(body), maxBodyBytes)
	}
}

// Endpoint already ending in "?" or "&" must not double-append a
// separator. Guards the HasSuffix branch in querySep.
func TestNew_EndpointTrailingSeparator(t *testing.T) {
	cases := []struct {
		name, endpoint string
	}{
		{"trailing_question", "https://example.com/wms?"},
		{"trailing_amp", "https://example.com/wms?map=foo&"},
	}
	for _, tc := range cases {
		src, err := New(Cfg{
			Endpoint:    tc.endpoint,
			Layers:      []string{"x"},
			Attribution: "© Example",
			MaxZoom:     18,
		})
		if err != nil {
			t.Fatalf("%s: New: %v", tc.name, err)
		}
		u := src.URL(tile.Coord{Z: 0, X: 0, Y: 0})
		// No doubled separators anywhere in the resulting URL.
		if strings.Contains(u, "??") || strings.Contains(u, "&&") ||
			strings.Contains(u, "?&") {
			t.Errorf("%s: URL has doubled separator: %s", tc.name, u)
		}
		// Query parses cleanly and carries the expected WMS params.
		q, perr := url.ParseQuery(strings.SplitN(u, "?", 2)[1])
		if perr != nil {
			t.Fatalf("%s: ParseQuery: %v", tc.name, perr)
		}
		if q.Get("service") != "WMS" {
			t.Errorf("%s: service = %q, want WMS", tc.name, q.Get("service"))
		}
	}
}

// Cfg.Concurrency=2 must allow at most 2 simultaneous in-flight requests.
func TestNew_ConcurrencyLimit(t *testing.T) {
	const limit = 2
	var inflight atomic.Int32
	var peak atomic.Int32
	gate := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			n := inflight.Add(1)
			defer inflight.Add(-1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			<-gate
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	cfg := validCfg()
	cfg.Endpoint = srv.URL
	cfg.Concurrency = limit
	src, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fetcher := src.(tile.HTTPFetcher).HTTPFetcher()

	errs := make(chan error, limit+1)
	for i := range limit + 1 {
		go func(i int) {
			resp, err := fetcher(context.Background(),
				srv.URL+"?bbox=0,0,1,1&i="+strconv.Itoa(i))
			if err == nil {
				_ = resp.Body.Close()
			}
			errs <- err
		}(i)
	}

	close(gate)
	for range limit + 1 {
		if err := <-errs; err != nil {
			t.Errorf("fetch error: %v", err)
		}
	}
	if p := peak.Load(); p > limit {
		t.Errorf("peak concurrent = %d, want <= %d", p, limit)
	}
}

func TestHTTPFetcher_RejectsServiceException(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			_, _ = io.WriteString(w,
				`<?xml version="1.0"?><ServiceExceptionReport/>`)
		}))
	defer srv.Close()

	src, _ := New(validCfg())
	f := src.(tile.HTTPFetcher).HTTPFetcher()
	_, err := f(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("err = nil, want rejection of XML body")
	}
}
