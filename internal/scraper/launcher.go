package scraper

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// launchBrowser resolves a Chromium binary via rod's launcher (which
// downloads a managed build on first use and caches it under
// ~/.cache/rod/browser), launches it headless, and connects a rod
// Browser to it. The browser lives until Close is called on the
// owning Client.
func launchBrowser() (*rod.Browser, error) {
	controlURL, err := launcher.New().
		Headless(true).
		NoSandbox(true).
		// Leakless spawns a small helper binary to force-kill the
		// browser when the Go process exits; on Windows Defender
		// flags that helper as malware, so we disable it. The
		// trade-off is that a hard Go-exit may leave a Chromium
		// process around until the next sweep.
		Leakless(false).
		Set("disable-gpu").
		Set("disable-dev-shm-usage").
		Set("disable-blink-features", "AutomationControlled").
		Launch()
	if err != nil {
		return nil, fmt.Errorf("scraper: launch chromium: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("scraper: connect to chromium: %w", err)
	}
	return browser, nil
}
