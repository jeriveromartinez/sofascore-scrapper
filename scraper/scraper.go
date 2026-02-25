package scraper

import (
	"context"
	"fmt"
	"log"
	"os"
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
			FilterFunction(func(_ int, s *goquery.Selection) bool {
				return s.Children().Filter("a[data-id]").Length() > 0
			}).
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
	rawText := strings.TrimSpace(s.Text())

	event := models.SportEvent{
		RawText:   rawText,
		ScrapedAt: time.Now(),
	}

	// Read the data-id from the direct child <a data-id> anchor.
	if dataID, exists := s.Children().Filter("a[data-id]").First().Attr("data-id"); exists {
		event.DataID = dataID
	}

	// Try to find team names: look for typical child elements that hold team names.
	teams := s.Find(`[class*="participant"], [class*="team"], bdi`).Map(func(_ int, sel *goquery.Selection) string {
		return strings.TrimSpace(sel.Text())
	})

	if len(teams) >= 2 {
		event.HomeTeam = teams[0]
		event.AwayTeam = teams[1]
	} else if len(teams) == 1 {
		event.HomeTeam = teams[0]
	}

	// Try to extract score information.
	scores := s.Find(`[class*="score"], [class*="Score"]`).Map(func(_ int, sel *goquery.Selection) string {
		return strings.TrimSpace(sel.Text())
	})

	if len(scores) >= 2 {
		event.HomeScore = scores[0]
		event.AwayScore = scores[1]
	} else if len(scores) == 1 {
		parts := strings.Split(scores[0], "-")
		if len(parts) == 2 {
			event.HomeScore = strings.TrimSpace(parts[0])
			event.AwayScore = strings.TrimSpace(parts[1])
		}
	}

	// Try to extract status / time.
	event.Status = strings.TrimSpace(s.Find(`[class*="status"], [class*="Status"], time`).First().Text())

	// Try to extract start time.
	event.StartTime = strings.TrimSpace(s.Find(`time, [class*="time"], [class*="Time"]`).First().Text())

	// Try to extract tournament/sport via aria-label or closest section header.
	event.Tournament = strings.TrimSpace(s.Find(`[class*="tournament"], [class*="league"], [class*="category"]`).First().Text())

	// Attempt to read sport from data-* attribute or aria attributes.
	if sport, exists := s.Attr("data-sport"); exists {
		event.Sport = sport
	}

	return event
}
