package imageproxy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultStoragePath = "./image_storage"

const imageDownloadTimeout = 10 * time.Second

// StoragePath returns the directory used to persist downloaded images.
// It can be overridden via the IMAGE_STORAGE_PATH environment variable.
func StoragePath() string {
	if p := os.Getenv("IMAGE_STORAGE_PATH"); p != "" {
		return p
	}
	return defaultStoragePath
}

// TeamLogoLocalPath returns the local filesystem path where a team logo is stored.
func TeamLogoLocalPath(teamID int64) string {
	return filepath.Join(StoragePath(), "teams", fmt.Sprintf("%d", teamID))
}

// TeamLogoAPIPath returns the API URL path that serves a team's proxied logo.
func TeamLogoAPIPath(teamID int64) string {
	return fmt.Sprintf("/api/v1/teams/logo/%d", teamID)
}

// DownloadTeamLogo fetches the image at sourceURL and saves it under the team
// logos storage directory.  If the file already exists it is a no-op.
// Returns the local filesystem path on success.
func DownloadTeamLogo(teamID int64, sourceURL string) (string, error) {
	localPath := TeamLogoLocalPath(teamID)

	// Already cached – nothing to do.
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("could not create image storage directory: %w", err)
	}

	client := &http.Client{Timeout: imageDownloadTimeout}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return "", fmt.Errorf("could not download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %d when downloading image", resp.StatusCode)
	}

	// Write to a temporary file first so a partial download never leaves a
	// corrupt file at the final path.
	tmpPath := localPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("could not create temp file: %w", err)
	}

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
		return "", fmt.Errorf("could not finalize image file: %w", err)
	}

	return localPath, nil
}
