package tile

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sync/semaphore"
)

// testSource returns an osmSource pointed at the given test-server
// URL with a fixed UA. Test-only helper; production callers use
// OSMWithUserAgent.
func testSource(t *testing.T, baseURL, ua string) *osmSource {
	t.Helper()
	prefix := baseURL + "/"
	return &osmSource{
		client:    http.DefaultClient,
		userAgent: SanitizeHeader(ua),
		urlPrefix: prefix,
		sem:       semaphore.NewWeighted(DefaultConcurrency),
	}
}

// pngFixture is the eight-byte PNG signature plus a short payload —
// enough to satisfy isPNG; tests never decode the bytes.
var pngFixture = []byte("\x89PNG\r\n\x1a\nfake-payload")

func TestOSM_URL(t *testing.T) {
	s := OSM()
	cases := []struct {
		name string
		c    Coord
		want string
	}{
		{
			"zero",
			Coord{Z: 0, X: 0, Y: 0},
			"https://tile.openstreetmap.org/0/0/0.png",
		},
		{
			"seattle_z11",
			Coord{Z: 11, X: 328, Y: 715},
			"https://tile.openstreetmap.org/11/328/715.png",
		},
		{
			"max_z19",
			Coord{Z: 19, X: 100, Y: 200},
			"https://tile.openstreetmap.org/19/100/200.png",
		},
	}
	for _, c := range cases {
		if got := s.URL(c.c); got != c.want {
			t.Errorf("%s: URL = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOSM_URL_InvalidCoord(t *testing.T) {
	s := OSM()
	// At zoom 2, max tile index is 3. X=4 is out of range.
	if got := s.URL(Coord{Z: 2, X: 4, Y: 0}); got != "" {
		t.Errorf("URL of invalid coord = %q, want \"\"", got)
	}
}

func TestOSM_HTTPFetcher_SendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	wantUA := "my-test-app/1.2 (https://example.com)"
	src, ok := OSMWithUserAgent(wantUA).(HTTPFetcher)
	if !ok {
		t.Fatal("OSMWithUserAgent does not implement HTTPFetcher")
	}
	fetcher := src.HTTPFetcher()
	resp, err := fetcher(context.Background(), srv.URL+"/1/2/3.png")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	_ = resp.Body.Close()

	if gotUA != wantUA {
		t.Errorf("User-Agent = %q, want %q", gotUA, wantUA)
	}
}

// URL must succeed at the maximum representable zoom (Z=31, where X/Y
// can be up to 10 digits) — exercises the stack-buffer sizing in
// buildTileURL.
func TestOSM_URL_MaxZoom31(t *testing.T) {
	s := OSM()
	c := Coord{Z: 31, X: (1 << 31) - 1, Y: (1 << 31) - 1}
	got := s.URL(c)
	want := "https://tile.openstreetmap.org/31/2147483647/2147483647.png"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

// At Z=32, uint32(1)<<32 = 0 in Go, so Coord.Valid returns false and
// URL must short-circuit to "" rather than build a nonsense address.
func TestOSM_URL_Z32IsInvalid(t *testing.T) {
	s := OSM()
	if got := s.URL(Coord{Z: 32, X: 0, Y: 0}); got != "" {
		t.Errorf("URL at Z=32 = %q, want \"\"", got)
	}
}

// SanitizeHeader must strip CR and LF so a malicious UA cannot append
// extra HTTP headers via injection.
func TestSanitizeHeader_StripsCRLF(t *testing.T) {
	got := SanitizeHeader("foo\r\nX-Inject: bar")
	want := "fooX-Inject: bar"
	if got != want {
		t.Errorf("SanitizeHeader = %q, want %q", got, want)
	}
}

func TestSanitizeHeader_TrimsWhitespace(t *testing.T) {
	got := SanitizeHeader("  hello  ")
	if got != "hello" {
		t.Errorf("SanitizeHeader = %q, want \"hello\"", got)
	}
}

// Length cap prevents an accidentally huge UA from landing on every
// outbound request.
func TestSanitizeHeader_CapsLength(t *testing.T) {
	in := strings.Repeat("a", MaxUserAgentLen+100)
	got := SanitizeHeader(in)
	if len(got) != MaxUserAgentLen {
		t.Errorf("len = %d, want %d", len(got), MaxUserAgentLen)
	}
}

func TestIsJPEG(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"too_short", []byte{0xFF, 0xD8}, false},
		{"jfif", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}, true},
		{"exif", []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00}, true},
		{"png", pngFixture, false},
	}
	for _, c := range cases {
		if got := IsJPEG(c.in); got != c.want {
			t.Errorf("%s: IsJPEG = %v, want %v", c.name, got, c.want)
		}
	}
}

// End-to-end: the sanitizer must run before the UA reaches outbound
// HTTP. Confirms construction wiring, not just the helper.
func TestOSMWithUserAgent_AppliesSanitizer(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("User-Agent")
		}))
	defer srv.Close()

	src := testSource(t, srv.URL, "evil\r\nX-Bad: yes")
	_, _ = src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})

	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("UA contains CRLF: %q", got)
	}
	if !strings.HasPrefix(got, "evil") {
		t.Errorf("UA = %q, want prefix \"evil\"", got)
	}
}

// LimitReader must cap response-body size; a hostile or
// misconfigured server returning gigabytes must not OOM the caller.
// Body is prefixed with a PNG signature so the post-LimitReader
// validator passes; the test then checks that the returned body
// stops at maxTileBytes.
func TestOSM_Fetch_LimitsResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pngMagic)
			big := make([]byte, maxTileBytes+(1<<20))
			_, _ = w.Write(big)
		}))
	defer srv.Close()

	src := testSource(t, srv.URL, "test/1")
	body, err := src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if int64(len(body)) > maxTileBytes {
		t.Errorf("body len = %d, exceeds cap %d", len(body), maxTileBytes)
	}
}

// HTTPFetcher must reject 200 OK responses whose body lacks a PNG
// signature — OSM occasionally returns empty bodies under load and
// go-gui would otherwise cache the garbage on disk.
func TestOSM_HTTPFetcher_RejectsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()

	src := OSMWithUserAgent("t/0").(HTTPFetcher)
	_, err := src.HTTPFetcher()(context.Background(), srv.URL+"/0/0/0.png")
	if err == nil {
		t.Fatal("err = nil, want rejection of empty body")
	}
	if !strings.Contains(err.Error(), "not a PNG") {
		t.Errorf("err = %v, want \"not a PNG\"", err)
	}
}

func TestOSM_HTTPFetcher_RejectsNonPNGBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<html>503 Backend Overloaded</html>")
		}))
	defer srv.Close()

	src := OSMWithUserAgent("t/0").(HTTPFetcher)
	_, err := src.HTTPFetcher()(context.Background(), srv.URL+"/0/0/0.png")
	if err == nil {
		t.Fatal("err = nil, want rejection of HTML body")
	}
	if !strings.Contains(err.Error(), "not a PNG") {
		t.Errorf("err = %v, want \"not a PNG\"", err)
	}
}

// HTTPFetcher must pass non-200 responses through unmodified so
// go-gui's existing status-code log fires; the body validator must
// not even run.
func TestOSM_HTTPFetcher_PassesThroughNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	defer srv.Close()

	src := OSMWithUserAgent("t/0").(HTTPFetcher)
	resp, err := src.HTTPFetcher()(context.Background(), srv.URL+"/0/0/0.png")
	if err != nil {
		t.Fatalf("err = %v, want nil for non-200 passthrough", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
}

func TestIsPNG(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"too_short", []byte{0x89, 'P', 'N'}, false},
		{"png", pngFixture, true},
		{"html", []byte("<html>"), false},
		{"jpeg_magic", []byte{0xFF, 0xD8, 0xFF, 0xE0}, false},
	}
	for _, c := range cases {
		if got := IsPNG(c.in); got != c.want {
			t.Errorf("%s: IsPNG = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOSM_Fetch_404ReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	defer srv.Close()

	src := testSource(t, srv.URL, "test/1")
	_, err := src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Non-404 error statuses must surface as a wrapped error with status
// info, not be swallowed.
func TestOSM_Fetch_500WrapsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer srv.Close()

	src := testSource(t, srv.URL, "test/1")
	_, err := src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if err == nil {
		t.Fatal("err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want to mention 500", err)
	}
}

// Sanity: the testSource helper itself must build URLs that the test
// server actually routes to. Catches mismatches between buildTileURL
// and the prefix-based override path.
func TestTestSource_FetchHitsRightPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src := testSource(t, srv.URL, "test/1")
	if _, err := src.Fetch(context.Background(),
		Coord{Z: 4, X: 5, Y: 6}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotPath != "/4/5/6.png" {
		t.Errorf("path = %q, want /4/5/6.png", gotPath)
	}
}

func TestValidateUserAgent(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid", "MyApp/1.0 (contact@example.com)", false},
		{"cr", "bad\rua", true},
		{"lf", "bad\nua", true},
		{"crlf", "bad\r\nua", true},
		{"exact_max", strings.Repeat("a", MaxUserAgentLen), false},
		{"over_max", strings.Repeat("a", MaxUserAgentLen+1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateUserAgent(c.in)
			if (err != nil) != c.wantErr {
				t.Errorf("ValidateUserAgent(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
			}
		})
	}
}

// OSMWithConfig(Concurrency:2) must allow at most 2 simultaneous requests.
func TestOSMWithConfig_ConcurrencyLimit(t *testing.T) {
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
			<-gate // hold until released
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pngFixture)
		}))
	defer srv.Close()

	src := OSMWithConfig(OSMConfig{
		UserAgent:   "test/1",
		Concurrency: limit,
	}).(*osmSource)
	src.urlPrefix = srv.URL + "/"

	fetcher := src.HTTPFetcher()
	errs := make(chan error, limit+1)
	for i := range limit + 1 {
		go func(i int) {
			resp, err := fetcher(context.Background(),
				srv.URL+"/1/0/"+strconv.Itoa(i)+".png")
			if err == nil {
				_ = resp.Body.Close()
			}
			errs <- err
		}(i)
	}

	// Release all requests.
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

func TestOSMWithConfig_DefaultConcurrency(t *testing.T) {
	src := OSMWithConfig(OSMConfig{}).(*osmSource)
	// With Concurrency=0, DefaultConcurrency slots must be available.
	if !src.sem.TryAcquire(DefaultConcurrency) {
		t.Fatalf("expected %d slots, could not acquire all", DefaultConcurrency)
	}
	src.sem.Release(DefaultConcurrency)
}

func TestOSMWithConfig_ZeroConcurrencyDefaults(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: 0}).(*osmSource)
	if !src.sem.TryAcquire(DefaultConcurrency) {
		t.Fatalf("Concurrency=0 should default to %d, got fewer slots", DefaultConcurrency)
	}
	src.sem.Release(DefaultConcurrency)
}

func TestOSMWithConfig_NegativeConcurrencyDefaults(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: -5}).(*osmSource)
	if !src.sem.TryAcquire(DefaultConcurrency) {
		t.Fatalf("Concurrency=-5 should default to %d, got fewer slots", DefaultConcurrency)
	}
	src.sem.Release(DefaultConcurrency)
}

func TestOSMWithConfig_PropagatesUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
		}))
	defer srv.Close()

	src := OSMWithConfig(OSMConfig{UserAgent: "TestApp/2.0"}).(*osmSource)
	src.urlPrefix = srv.URL + "/"
	_, _ = src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if gotUA != "TestApp/2.0" {
		t.Errorf("UA = %q, want TestApp/2.0", gotUA)
	}
}

func TestOSMWithConfig_EmptyUADefaultsToGoMap(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
		}))
	defer srv.Close()

	src := OSMWithConfig(OSMConfig{}).(*osmSource)
	src.urlPrefix = srv.URL + "/"
	_, _ = src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if !strings.HasPrefix(gotUA, "go-map/") {
		t.Errorf("default UA = %q, want go-map/ prefix", gotUA)
	}
}

func TestOSMWithConfig_ConcurrencyFive(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: 5}).(*osmSource)
	if !src.sem.TryAcquire(5) {
		t.Fatal("Concurrency=5: expected 5 slots")
	}
	if src.sem.TryAcquire(1) {
		t.Error("Concurrency=5: acquired 6th slot, want blocked")
		src.sem.Release(1)
	}
	src.sem.Release(5)
}

func TestOSMWithConfig_PropagatesContext(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: 1}).(*osmSource)
	// Fill the semaphore so the next acquire must wait.
	if !src.sem.TryAcquire(1) {
		t.Fatal("could not acquire initial slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Fetch with a pre-cancelled context must return promptly.
	_, err := src.Fetch(ctx, Coord{Z: 1, X: 0, Y: 0})
	src.sem.Release(1)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestOSMWithConfig_HTTPFetcherPropagatesContext(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: 1}).(*osmSource)
	if !src.sem.TryAcquire(1) {
		t.Fatal("could not acquire initial slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetcher := src.HTTPFetcher()
	_, err := fetcher(ctx, "http://localhost/0/0/0.png")
	src.sem.Release(1)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestOSMWithConfig_ContextCancelledDuringFetch(t *testing.T) {
	src := OSMWithConfig(OSMConfig{Concurrency: 2})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Fetch(ctx, Coord{Z: 1, X: 0, Y: 0})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestOSMWithConfig_FetchInvalidCoord(t *testing.T) {
	src := OSMWithConfig(OSMConfig{})
	_, err := src.Fetch(context.Background(), Coord{Z: 32, X: 0, Y: 0})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("invalid coord: err = %v, want ErrNotFound", err)
	}
}

func TestOSMWithConfig_URLInvalidCoord(t *testing.T) {
	src := OSMWithConfig(OSMConfig{})
	if url := src.URL(Coord{Z: 32, X: 0, Y: 0}); url != "" {
		t.Errorf("URL of invalid coord = %q, want \"\"", url)
	}
}

func TestOSMWithConfig_Attribution(t *testing.T) {
	src := OSMWithConfig(OSMConfig{})
	if src.Attribution() == "" {
		t.Error("Attribution() empty")
	}
}

func TestOSMWithConfig_MaxZoom(t *testing.T) {
	src := OSMWithConfig(OSMConfig{})
	if src.MaxZoom() == 0 {
		t.Error("MaxZoom() == 0")
	}
}

func TestOSMWithConfig_ImplementsHTTPFetcher(t *testing.T) {
	src := OSMWithConfig(OSMConfig{})
	if _, ok := src.(HTTPFetcher); !ok {
		t.Error("OSMWithConfig result does not implement HTTPFetcher")
	}
}

func TestOSMWithConfig_SanitizesUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
		}))
	defer srv.Close()

	src := OSMWithConfig(OSMConfig{UserAgent: "evil\r\nX-Bad: yes"}).(*osmSource)
	src.urlPrefix = srv.URL + "/"
	_, _ = src.Fetch(context.Background(), Coord{Z: 1, X: 0, Y: 0})
	if strings.ContainsAny(gotUA, "\r\n") {
		t.Errorf("UA contains CRLF: %q", gotUA)
	}
}

func TestOSM_HTTPFetcher_PropagatesContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()

	src := OSMWithUserAgent("t/0").(HTTPFetcher)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call
	_, err := src.HTTPFetcher()(ctx, srv.URL+"/0/0/0.png")
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") &&
		!strings.Contains(err.Error(), "context") {
		t.Errorf("error = %v, want context-cancellation", err)
	}
}
