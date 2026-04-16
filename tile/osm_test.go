package tile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
