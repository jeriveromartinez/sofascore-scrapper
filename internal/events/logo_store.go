package events

import (
	"context"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultStoragePath = "./image_storage"

const imageDownloadTimeout = 10 * time.Second

const imageBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

// newImageHTTPClient returns the HTTP client used to download team
// logos. It uses the standard library's *http.Transport so the Go
// runtime negotiates TLS 1.2/1.3 with its stable, Cloudflare/CloudFront-
// accepted fingerprint.
//
// Why not uTLS: the previous implementation forced TLS 1.2 with
// utls.HelloRandomizedALPN (a random JA3 fingerprint). Both the
// img.sofascore.com and images.fotmob.com CDNs sit behind Cloudflare/
// CloudFront, which rejects randomized JA3s with HTTP 403. Forcing
// TLS 1.2 also broke against any CDN that had deprecated TLS 1.0/1.1.
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

func StoragePath() string {
	if p := os.Getenv("IMAGE_STORAGE_PATH"); p != "" {
		return p
	}
	return defaultStoragePath
}

func TeamLogoLocalPath(teamID int64) string {
	return filepath.Join(StoragePath(), "teams", fmt.Sprintf("%d", teamID))
}

func TeamLogoAPIPath(teamID int64) string {
	return fmt.Sprintf("/teams/logo/%d", teamID)
}

func TeamLogoSourceURL(teamID int64) string {
	return fmt.Sprintf("https://img.sofascore.com/api/v1/team/%d/image", teamID)
}

func DownloadTeamLogo(teamID int64, sourceURL string) (string, error) {
	return DownloadTeamLogoWithContext(context.Background(), teamID, sourceURL)
}

func DownloadTeamLogoWithContext(ctx context.Context, teamID int64, sourceURL string) (string, error) {
	return downloadTeamLogoWithContext(ctx, teamID, sourceURL, newImageHTTPClient())
}

func downloadTeamLogo(teamID int64, sourceURL string, client *http.Client) (string, error) {
	return downloadTeamLogoWithContext(context.Background(), teamID, sourceURL, client)
}

func downloadTeamLogoWithContext(ctx context.Context, teamID int64, sourceURL string, client *http.Client) (string, error) {
	defer client.CloseIdleConnections()

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
	// CDN's expectation. FotMob and SofaScore CDNs both reject requests
	// with the wrong (or empty) Referer. Using the URL origin keeps the
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

	f, err := os.CreateTemp(dir, "logo-*.tmp")
	if err != nil {
		return "", fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := f.Name()

	if _, copyErr := io.Copy(f, resp.Body); copyErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("could not write image data: %w", copyErr)
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
