package scores365

import (
	"strings"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
)

// sportMap converts a 365scores sportId into the canonical sport
// slug used by `scraper_leagues.sport` and `events.sport`.
//
// The IDs come from /web/sports/ (see FetchSports). Two slugs
// deliberately differ from the API's own nameForURL because the
// canonical vocabulary in this project predates the migration:
// sportId 4 is "hockey" upstream but "ice-hockey" here, and
// sportId 6 is "a.-football" upstream but "american-football" here.
var sportMap = map[int]string{
	1: "football",
	2: "basketball",
	3: "tennis",
	4: "ice-hockey",
	5: "handball",
	6: "american-football",
	7: "baseball",
	8: "volleyball",
	9: "rugby",
}

// sportIDMap is the inverse of sportMap, used by the catalog
// discovery path to turn a configured slug list into the numeric
// ?sports= filter the competitions endpoint requires.
var sportIDMap = func() map[string]int {
	out := make(map[string]int, len(sportMap))
	for id, slug := range sportMap {
		out[slug] = id
	}
	// Accept the upstream spellings too so operators can configure
	// either vocabulary.
	out["hockey"] = 4
	out["a.-football"] = 6
	return out
}()

func sportSlugFromID(sportID int) string {
	if s, ok := sportMap[sportID]; ok {
		return s
	}
	return "unknown"
}

// SportSlug exposes the sportId → slug mapping to the catalog
// discovery package.
func SportSlug(sportID int) string { return sportSlugFromID(sportID) }

// SportIDForSlug resolves a canonical (or upstream) sport slug to
// the numeric 365scores sportId. Returns 0 when the slug is unknown.
func SportIDForSlug(slug string) int {
	return sportIDMap[strings.ToLower(strings.TrimSpace(slug))]
}

// statusKind is the coarse lifecycle bucket a fixture is in, derived
// from the payload's statusGroup plus its (localized) statusText.
type statusKind int

const (
	statusScheduled statusKind = iota
	statusLive
	statusFinished
	statusCancelled
)

// abandonedKeywords are substrings that mark a fixture as "never
// produced a result" regardless of which statusGroup it is reported
// under. 365scores only exposes the fine-grained status as a
// *localized* string (the legacy /data/games numeric GT code is gone
// from the web API), so keyword matching is the only way to tell a
// cancelled or postponed fixture apart from a finished one — both
// arrive as statusGroup 4.
//
// Both the es-mx labels (the default langId) and the English ones
// are listed so a deployment that overrides SCORES365_LANG_ID still
// classifies correctly. Matching is done on a lowercased, accent-
// folded copy of the text, so "Aplazado" and "aplazado" both hit.
var abandonedKeywords = []string{
	// es-mx
	"cancelado",
	"aplazado",
	"pospuesto",
	"suspendido",
	"abandonado",
	"anulado",
	// en
	"cancelled",
	"canceled",
	"postponed",
	"suspended",
	"abandoned",
	"called off",
}

// classify maps a wire Game onto a statusKind.
//
// statusGroup is the authoritative coarse bucket:
//
//	2 -> scheduled
//	3 -> live
//	4 -> ended
//
// A cancelled/postponed fixture is reported as group 4 with a
// statusText naming the reason, so group 4 is refined by keyword
// before being accepted as "finished". Group 2 is refined the same
// way because a fixture postponed before kickoff can linger there.
func classify(g scores365.Game) statusKind {
	if hasAbandonedKeyword(g.StatusText) || hasAbandonedKeyword(g.ShortStatusText) {
		return statusCancelled
	}
	switch g.StatusGroup {
	case scores365.StatusGroupLive:
		return statusLive
	case scores365.StatusGroupEnded:
		return statusFinished
	case scores365.StatusGroupScheduled:
		return statusScheduled
	}
	// Unknown group: fall back to scheduled so the row is persisted
	// as notstarted rather than being reported as a result that
	// never happened.
	return statusScheduled
}

func hasAbandonedKeyword(text string) bool {
	if text == "" {
		return false
	}
	norm := foldAccents(strings.ToLower(text))
	for _, kw := range abandonedKeywords {
		if strings.Contains(norm, kw) {
			return true
		}
	}
	return false
}

// foldAccents strips the handful of Latin-1 diacritics that appear in
// 365scores' Spanish status labels so the keyword table can be
// written unaccented. It is deliberately not a full Unicode NFD fold:
// the input set is small and fixed.
var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

func foldAccents(s string) string { return accentFolder.Replace(s) }
