package scraper

import (
	"fmt"
	"os"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// launchBrowser resolves a Chromium binary via rod's launcher and
// launches it headless. When rodBrowserBin is set (typically via the
// SOFASCRAPER_CHROMIUM_BIN env var, populated in the Dockerfile to
// /usr/bin/chromium), it is used as-is and the auto-download path is
// skipped. Otherwise rod downloads its pinned build on first launch.
//
// When SOFASCRAPER_PROXY_URL is set, Chromium is launched with
// --proxy-server=<url>. Accepts any URL Chromium's proxy-server flag
// understands: socks5://user:pass@host:port (DNS resolved locally) or
// socks5h://user:pass@host:port (DNS resolved via proxy — preferred
// when the goal is hiding the resolver IP from the target). Empty
// means no proxy and the browser uses the host network directly.
//
// The browser lives until Close is called on the owning Client.
func launchBrowser() (*rod.Browser, error) {
	l := launcher.New().
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
		// --enable-automation is the default flag Chromium adds when
		// it detects CDP control. Many bot-detection systems (incl.
		// Fastly's WAF) treat it as an outright block signal. rod's
		// NewUserMode removes it; we do the same here for headless.
		Delete("enable-automation")

	if bin := os.Getenv("SOFASCRAPER_CHROMIUM_BIN"); bin != "" {
		l = l.Bin(bin)
	}

	if proxy := os.Getenv("SOFASCRAPER_PROXY_URL"); proxy != "" {
		l = l.Proxy(proxy)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("scraper: launch chromium: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("scraper: connect to chromium: %w", err)
	}
	return browser, nil
}
