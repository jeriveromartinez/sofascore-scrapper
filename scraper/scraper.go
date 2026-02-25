package scraper

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

const (
	sofascoreURL = "https://www.sofascore.com/es/"
	waitTimeout  = 30 * time.Second
	parentClass  = "mdDown:pt_sm"
	eventClass   = "debpTI"
)

// Scrape fetches sports events from Sofascore using a headless browser,
// parses elements with class "debpTI" (that have a direct child <a data-id>)
// inside elements with class "mdDown:pt_sm", and returns a slice of SportEvent.
func Scrape() ([]models.SportEvent, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	if browserPath := resolveBrowserExecPath(); browserPath != "" {
		opts = append(opts, chromedp.ExecPath(browserPath))
	}

	// --no-sandbox is required in some container environments (e.g. Docker without
	// a user namespace). Enable it by setting CHROMIUM_NO_SANDBOX=true.
	if os.Getenv("CHROMIUM_NO_SANDBOX") == "true" {
		opts = append(opts, chromedp.Flag("no-sandbox", true))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, waitTimeout)
	defer cancelTimeout()

	var pageHTML string

	// CSS selector that waits for an event element to actually appear in the DOM,
	// avoiding a fixed sleep and still handling slow page loads gracefully.
	eventSelector := fmt.Sprintf(`[class*="%s"] [class*="%s"]`, parentClass, eventClass)

	err := chromedp.Run(ctx,
		chromedp.Navigate(sofascoreURL),
		// Wait until at least one event element is visible.
		chromedp.WaitVisible(eventSelector, chromedp.ByQuery),
		chromedp.OuterHTML("html", &pageHTML),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching page with chromedp: %w", err)
	}

	return parseEvents(pageHTML)
}

func resolveBrowserExecPath() string {
	if envPath := strings.TrimSpace(os.Getenv("CHROMEDP_EXEC_PATH")); envPath != "" {
		return envPath
	}

	candidates := []string{
		"google-chrome",
		"chrome",
		"chromium",
		"chromium-browser",
		"brave-browser",
		"brave-browser-stable",
		"brave",
	}

	if runtime.GOOS == "windows" {
		programFiles := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")}
		for _, base := range programFiles {
			if base == "" {
				continue
			}

			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			continue
		}

		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}

	return ""
}

// parseEvents parses the raw HTML and extracts sports events.
// Only "debpTI" elements that have a direct child <a> with a "data-id" attribute are processed.
func parseEvents(pageHTML string) ([]models.SportEvent, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %w", err)
	}

	var events []models.SportEvent

	// Find all parent elements that have the class "mdDown:pt_sm".
	doc.Find(fmt.Sprintf(`[class*="%s"]`, parentClass)).Each(func(i int, parent *goquery.Selection) {
		// Inside each parent, find child elements with class "debpTI" that also
		// have a direct child <a> with a "data-id" attribute.
		parent.Find(fmt.Sprintf(`[class*="%s"]`, eventClass)).
			FilterFunction(func(_ int, s *goquery.Selection) bool { return s.Children().Filter("a[data-id]").Length() > 0 }).
			Each(func(j int, s *goquery.Selection) {
				event := extractEvent(s)
				if event.RawText != "" {
					events = append(events, event)
				}
			})
	})

	log.Printf("Parsed %d events from page.", len(events))
	return events, nil
}

// extractEvent extracts sport event data from a single event element.
func extractEvent(s *goquery.Selection) models.SportEvent {
	event := models.SportEvent{ScrapedAt: time.Now(), RawText: strings.TrimSpace(s.Text())}

	// Read the data-id from the direct child <a data-id> anchor.
	anchor := s.Children().Filter("a[data-id]").First()
	if dataID, exists := anchor.Attr("data-id"); exists {
		event.DataID = dataID
	}

	// Team names come from the alt attribute of <img> tags inside the event.
	// Image URLs are the src attribute with the trailing "/small" segment removed.
	anchor.Find("img[alt]").Each(func(i int, img *goquery.Selection) {
		alt := strings.TrimSpace(img.AttrOr("alt", ""))
		src := strings.TrimSpace(img.AttrOr("src", ""))
		// Strip the "/small" suffix from the image URL.
		imgURL := strings.TrimSuffix(src, "/small")

		switch i {
		case 0:
			event.HomeTeam = alt
			event.HomeTeamImage = imgURL
		case 1:
			event.AwayTeam = alt
			event.AwayTeamImage = imgURL
		}
	})

	// Extract scores: Sofascore renders each score value in its own element.
	// We look for bdi elements that contain only digit characters.
	var scoreValues []string
	anchor.Find("bdi").Each(func(_ int, bdi *goquery.Selection) {
		txt := strings.TrimSpace(bdi.Text())
		if isScore(txt) {
			scoreValues = append(scoreValues, txt)
		}
	})
	if len(scoreValues) >= 2 {
		event.HomeScore = scoreValues[0]
		event.AwayScore = scoreValues[1]
	}

	// Extract start time (pre-match) and in-play status from bdi text colour classes.
	s.Find(`bdi[class*="c_neutrals.nLv3"]`).Each(func(_ int, bdi *goquery.Selection) {
		if txt := strings.TrimSpace(bdi.Text()); txt != "" && event.StartTime == "" {
			event.StartTime = txt
		}
	})
	s.Find(`bdi[class*="c_neutrals.nLv1"]`).Each(func(_ int, bdi *goquery.Selection) {
		if txt := strings.TrimSpace(bdi.Text()); txt != "" && event.Status == "" {
			event.Status = txt
		}
	})

	// Try to extract tournament / sport from the enclosing section if present.
	event.Tournament = strings.TrimSpace(s.Find(`[class*="tournament"], [class*="league"], [class*="category"]`).First().Text())
	if sport, exists := s.Attr("data-sport"); exists {
		event.Sport = sport
	}

	return event
}

// isScore reports whether s looks like a numeric score value (digits only, optionally
// representing extra time or penalty scores such as "0", "2", "10").
func isScore(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
