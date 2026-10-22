package events

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
)

const defaultStoragePath = "./image_storage"

const imageDownloadTimeout = 10 * time.Second

// maxLogoBytes caps the body the downloader will read from a remote
// CDN. A misconfigured upstream (or a MITM-injected payload) could
// otherwise push the body to multi-GB and fill the disk. 4 MiB is
// well above any legitimate team badge; legitimate PNG/SVG/WebP
// variants stay under 200 KiB.
const maxLogoBytes int64 = 4 << 20

const imageBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

// newImageHTTPClient returns the HTTP client used to download team
// logos. It uses the standard library's *http.Transport so the Go
// runtime negotiates TLS 1.2/1.3 with its stable, Cloudflare/CloudFront-
// accepted fingerprint.
//
// Why not uTLS: the previous implementation forced TLS 1.2 with
// utls.HelloRandomizedALPN (a random JA3 fingerprint). Both the
// imagecache.365scores.com and images.fotmob.com CDNs sit behind
// Cloudflare/CloudFront, which rejects randomized JA3s with HTTP 403.
// Forcing TLS 1.2 also broke against any CDN that had deprecated
// TLS 1.0/1.1.
// Since CDN image endpoints do not require impersonation (they only
// check the Referer / User-Agent, both of which the client sends
// explicitly), the default transport is both simpler and reliable.
func newImageHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   imageDownloadTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: imageDownloadTimeout,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   imageDownloadTimeout,
	}
}

// defaultImageClient is the package-wide shared client the logo
// downloaders use. The LogoScheduler fans out hundreds of downloads
// per scrape (one HTTP call per team per source URL); allocating a
// fresh *http.Transport per call destroyed connection reuse and
// burned one TCP+TLS handshake per call. Sharing one client across
// the whole scheduler lets idle connections stay warm between jobs.
var (
	defaultImageClientOnce sync.Once
	defaultImageClient     *http.Client
)

func sharedImageHTTPClient() *http.Client {
	defaultImageClientOnce.Do(func() {
		defaultImageClient = newImageHTTPClient()
	})
	return defaultImageClient
}

func StoragePath() string {
	if p := os.Getenv("IMAGE_STORAGE_PATH"); p != "" {
		return p
	}
	return defaultStoragePath
}

func TeamLogoLocalPath(teamID int64) string {
	return filepath.Join(StoragePath(), "teams", fmt.Sprintf("%d", teamID))
}

const teamLogoAPIPrefix = "/teams/logo/"

func TeamLogoAPIPath(teamID int64) string {
	return fmt.Sprintf("%s%d", teamLogoAPIPrefix, teamID)
}

// Source-ID namespaces in the shared `teams` table. IDs are prefixed
// per source so two upstreams can never collide on the primary key.
// FotMob rows keep their natural ID (their IDs go up to ~2M), so any
// ID below the lowest prefix is a FotMob row.
const (
	// TeamIDPrefixScores365 mirrors
	// internal/scraper/scores365.TeamIDPrefix.
	TeamIDPrefixScores365 int64 = 7_000_000_000
	// TeamIDPrefixLegacySportsDB is the namespace of the removed
	// TheSportsDB source. Rows in this range may still exist in
	// long-lived databases; there is no upstream left to download
	// their badges from.
	TeamIDPrefixLegacySportsDB int64 = 2_000_000_000
)

// fotmobTeamLogoURL is the badge pattern on FotMob's CDN.
//
// Note the path uses a slash, not an underscore:
// .../teamlogo/<id>.png returns the PNG, while the
// .../teamlogo_<id>.png spelling an earlier revision used is rejected
// by the CloudFront distribution in front of the bucket with 403 for
// every Referer. That 403 is why this fallback used to point at a
// third-party CDN instead.
const fotmobTeamLogoURL = "https://images.fotmob.com/image_resources/logo/teamlogo/%d.png"

// TeamLogoSourceURL returns the URL the LogoScheduler should use as a
// last-resort fallback when neither the source-provided PrimaryURL
// nor the LogoLookup resolve the team's logo. It is the single source
// of truth for that mapping; scraper.LogoURLForSource delegates here.
//
// The URL is derived from the ID namespace, so each team's badge is
// fetched from the upstream that published the team:
//
//	>= 7_000_000_000  -> 365scores image CDN (prefix stripped)
//	>= 2_000_000_000  -> removed TheSportsDB namespace, no URL
//	otherwise         -> FotMob CDN (natural ID)
//
// Stripping the prefix matters: both CDNs are keyed by the upstream's
// natural ID, so a 7_000_000_000+ ID sent verbatim resolves to
// nothing.
//
// Returns "" when no upstream can serve the badge; callers must treat
// an empty URL as "nothing to download" rather than requesting it.
func TeamLogoSourceURL(teamID int64) string {
	switch {
	case teamID >= TeamIDPrefixScores365:
		return scores365.CompetitorImageURL(int(teamID-TeamIDPrefixScores365), 0)
	case teamID >= TeamIDPrefixLegacySportsDB:
		return ""
	case teamID <= 0:
		return ""
	default:
		return fmt.Sprintf(fotmobTeamLogoURL, teamID)
	}
}

func DownloadTeamLogo(teamID int64, sourceURL string) (string, error) {
	return DownloadTeamLogoWithContext(context.Background(), teamID, sourceURL)
}

// DownloadTeamLogoWithContext is the production entry point. It uses
// the package-level shared HTTP client so the LogoScheduler's many
// concurrent downloads reuse idle keep-alive connections instead of
// each spinning up its own TCP+TLS handshake.
func DownloadTeamLogoWithContext(ctx context.Context, teamID int64, sourceURL string) (string, error) {
	return downloadTeamLogoWithContext(ctx, teamID, sourceURL, sharedImageHTTPClient())
}

// downloadTeamLogo is the test seam — it accepts an explicit client
// so tests can inject httptest server clients and assert on transport
// behaviour. Production callers should go through DownloadTeamLogo /
// DownloadTeamLogoWithContext.
func downloadTeamLogo(teamID int64, sourceURL string, client *http.Client) (string, error) {
	return downloadTeamLogoWithContext(context.Background(), teamID, sourceURL, client)
}

func downloadTeamLogoWithContext(ctx context.Context, teamID int64, sourceURL string, client *http.Client) (string, error) {
	// The shared production client is reused across every download on
	// the scheduler; closing its idle pool per-call would defeat the
	// connection reuse this refactor exists to enable. Test-injected
	// one-off clients are still closed so the test seam can assert
	// clean teardown.
	if client != sharedImageHTTPClient() {
		defer client.CloseIdleConnections()
	}

	// TeamLogoSourceURL returns "" for IDs no upstream can serve
	// (the removed TheSportsDB namespace). Fail fast instead of
	// issuing a request against an empty URL.
	if sourceURL == "" {
		return "", fmt.Errorf("no logo source URL for team %d", teamID)
	}

	localPath := TeamLogoLocalPath(teamID)

	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create image storage directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not create image request: %w", err)
	}
	req.Header.Set("User-Agent", imageBrowserUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	// Use the source URL's origin as Referer so the request matches the
	// CDN's expectation. The FotMob and 365scores CDNs both reject
	// requests with the wrong (or empty) Referer. Using the URL origin keeps the
	// header correct regardless of which upstream CDN the team row came
	// from. If parsing fails, fall back to the origin path of sourceURL.
	if u, parseErr := neturl.Parse(sourceURL); parseErr == nil && u.Scheme != "" && u.Host != "" {
		req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %d when downloading image", resp.StatusCode)
	}

	// Validate Content-Type before writing anything to disk. Some CDNs
	// return a small HTML "challenge" or JSON error body with HTTP 200
	// when the request is rate-limited or geo-blocked; persisting that
	// as a logo would silently break every team page that referenced
	// it.
	//
	// What we accept:
	//   - empty Content-Type (some CDNs omit the header for images)
	//   - image/* (the normal case)
	//   - application/octet-stream (Go's stdlib sniffs high-bit binary
	//     blobs that don't match a known image signature into this
	//     type; legitimate CDNs sometimes serve a logo this way)
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") && !strings.EqualFold(ct, "application/octet-stream") {
		return "", fmt.Errorf("unexpected Content-Type %q for team %d", ct, teamID)
	}

	// Reject obviously oversized responses before reading the body so
	// we don't allocate a multi-GB buffer for a hostile or buggy
	// upstream. ContentLength is best-effort (chunked transfer has it
	// as -1); the LimitReader below is the belt-and-suspenders cap.
	if resp.ContentLength > maxLogoBytes {
		return "", fmt.Errorf("logo body too large: Content-Length=%d for team %d", resp.ContentLength, teamID)
	}

	f, err := os.CreateTemp(dir, "logo-*.tmp")
	if err != nil {
		return "", fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := f.Name()

	// Read one byte past the cap. io.LimitReader alone would stop at
	// exactly maxLogoBytes and silently persist the truncated prefix
	// as if it were a complete logo, so a chunked response (no
	// Content-Length to check up front) would slip through.
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxLogoBytes+1))
	if copyErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("could not write image data: %w", copyErr)
	}
	if written > maxLogoBytes {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("logo body too large: more than %d bytes for team %d", maxLogoBytes, teamID)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("could not flush image data: %w", err)
	}

	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		if _, statErr := os.Stat(localPath); statErr == nil {
			return localPath, nil
		}
		return "", fmt.Errorf("could not finalize image file: %w", err)
	}

	return localPath, nil
}
