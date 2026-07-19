package events

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	utls "github.com/refraction-networking/utls"
)

const defaultStoragePath = "./image_storage"

const imageDownloadTimeout = 10 * time.Second

const imageBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

func newImageHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DialTLSContext: dialImageTLS},
		Timeout:   imageDownloadTimeout,
	}
}

func dialImageTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, _ := net.SplitHostPort(addr)
	raw, err := (&net.Dialer{Timeout: imageDownloadTimeout}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	conn := utls.UClient(raw, &utls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	}, utls.HelloRandomized)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return conn, nil
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

func DownloadTeamLogo(teamID int64, sourceURL string) (string, error) {
	localPath := TeamLogoLocalPath(teamID)

	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create image storage directory: %w", err)
	}

	client := newImageHTTPClient()
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not create image request: %w", err)
	}
	req.Header.Set("User-Agent", imageBrowserUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://www.sofascore.com/")

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
