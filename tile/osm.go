package tile

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// osmSource fetches tiles from the public OSM tile server.
// Usage requires compliance with
// https://operations.osmfoundation.org/policies/tiles/
//
// Throttling is the caller's responsibility: OSM tile policy bans
// heavy or bulk traffic. This source does not rate-limit.
type osmSource struct {
	client    *http.Client
	userAgent string
	urlPrefix string // per-server URL stem; overridable in tests.
}

// osmURLPrefix is the production OSM tile-server URL stem.
const osmURLPrefix = "https://tile.openstreetmap.org/"

// maxTileBytes caps response-body size per fetch. A 256² tile is a
// few KiB; the cap stops a hostile or misconfigured server from
// driving io.ReadAll into unbounded memory.
const maxTileBytes int64 = 4 << 20 // 4 MiB

// OSM returns a tile source backed by the public OpenStreetMap tile
// server. The caller must set a descriptive User-Agent via
// OSMWithUserAgent for production use; heavy/anonymous traffic is
// blocked by OSM policy.
func OSM() Source {
	return OSMWithUserAgent("go-map/0 (https://github.com/mike-ward/go-map)")
}

// OSMWithUserAgent returns an OSM tile source using the given
// User-Agent string. CR and LF are stripped to prevent header
// injection at write time (net/http would otherwise reject the
// request at runtime).
func OSMWithUserAgent(ua string) Source {
	return &osmSource{
		client:    &http.Client{Timeout: 15 * time.Second},
		userAgent: sanitizeHeader(ua),
		urlPrefix: osmURLPrefix,
	}
}

// maxUserAgentLen caps the length of the User-Agent we will set on
// outbound requests. RFC 7230 doesn't fix a header-value ceiling but
// many origins (and Go's own http2 stack) reject very long values;
// keep us defensive against accidental megabyte strings.
const maxUserAgentLen = 512

// sanitizeHeader strips characters that would make net/http reject a
// header value (\r, \n), trims surrounding whitespace, and caps the
// length so a hostile or malformed caller cannot inject an arbitrary
// blob into every outbound request.
func sanitizeHeader(s string) string {
	r := strings.NewReplacer("\r", "", "\n", "")
	out := strings.TrimSpace(r.Replace(s))
	if len(out) > maxUserAgentLen {
		out = out[:maxUserAgentLen]
	}
	return out
}

// buildTileURL composes "{prefix}{z}/{x}/{y}.png" without going
// through fmt: one allocation for the returned string vs ~5 for
// fmt.Sprintf, on every visible tile every frame.
func buildTileURL(prefix string, c Coord) string {
	var buf [128]byte
	b := append(buf[:0], prefix...)
	b = strconv.AppendUint(b, uint64(c.Z), 10)
	b = append(b, '/')
	b = strconv.AppendUint(b, uint64(c.X), 10)
	b = append(b, '/')
	b = strconv.AppendUint(b, uint64(c.Y), 10)
	b = append(b, ".png"...)
	return string(b)
}

func (s *osmSource) URL(c Coord) string {
	if !c.Valid() {
		return ""
	}
	return buildTileURL(s.urlPrefix, c)
}

func (s *osmSource) Fetch(ctx context.Context, c Coord) ([]byte, error) {
	if !c.Valid() {
		return nil, ErrNotFound
	}
	url := buildTileURL(s.urlPrefix, c)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(io.LimitReader(resp.Body, maxTileBytes))
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("tile: %s: %s", url, resp.Status)
	}
}

func (s *osmSource) Attribution() string {
	return "© OpenStreetMap contributors"
}

func (*osmSource) MaxZoom() uint32 { return 19 }

// HTTPFetcher returns a function suitable for
// gui.WindowCfg.ImageFetcher. It sends the Source's User-Agent on
// every request — required by OSM tile policy when rendering via
// gui.DrawContext.Image. The response body is wrapped with
// io.LimitReader(maxTileBytes) so a hostile or misconfigured server
// cannot drive the renderer's image decoder into unbounded reads —
// the same cap Fetch enforces.
func (s *osmSource) HTTPFetcher() func(ctx context.Context, url string) (*http.Response, error) {
	return func(ctx context.Context, url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", s.userAgent)
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		resp.Body = limitedBody(resp.Body, maxTileBytes)
		return resp, nil
	}
}

// limitedBody wraps an io.ReadCloser so the consumer cannot read
// past n bytes. Close still hits the original body so the
// underlying connection can be released to the http.Client pool.
func limitedBody(rc io.ReadCloser, n int64) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(rc, n), rc}
}

// HTTPFetcher is implemented by Sources that speak HTTP and can
// supply a policy-compliant fetcher for gui.WindowCfg.ImageFetcher.
// Consumers type-assert to this interface.
type HTTPFetcher interface {
	HTTPFetcher() func(ctx context.Context, url string) (*http.Response, error)
}
