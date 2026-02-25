package scraper

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"golang.org/x/net/html"
)

const (
	sofascoreURL   = "https://www.sofascore.com/es/"
	waitTimeout    = 30 * time.Second
	parentClass    = "mdDown:pt_sm"
	eventClass     = "debpTI"
)

// Scrape fetches sports events from Sofascore using a headless browser,
// parses elements with class "debpTI" inside elements with class "mdDown:pt_sm",
// and returns a slice of SportEvent.
func Scrape() ([]models.SportEvent, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, waitTimeout)
	defer cancelTimeout()

	var pageHTML string

	err := chromedp.Run(ctx,
		chromedp.Navigate(sofascoreURL),
		// Wait for the main content to be available.
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		// Give JS a moment to render dynamic content.
		chromedp.Sleep(5*time.Second),
		chromedp.OuterHTML("html", &pageHTML),
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching page with chromedp: %w", err)
	}

	return parseEvents(pageHTML)
}

// parseEvents parses the raw HTML and extracts sports events.
func parseEvents(pageHTML string) ([]models.SportEvent, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %w", err)
	}

	var events []models.SportEvent

	// Find all parent elements that have the class "mdDown:pt_sm".
	doc.Find(fmt.Sprintf(`[class*="%s"]`, parentClass)).Each(func(i int, parent *goquery.Selection) {
		// Inside each parent, find child elements with class "debpTI".
		parent.Find(fmt.Sprintf(`[class*="%s"]`, eventClass)).Each(func(j int, s *goquery.Selection) {
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

// nodeToText is a helper that recursively extracts text from an html.Node.
func nodeToText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(nodeToText(c))
	}
	return sb.String()
}
