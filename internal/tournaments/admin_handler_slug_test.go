package tournaments

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

func postTournament(t *testing.T, r http.Handler, req *pb.TournamentRequest) (*httptest.ResponseRecorder, *pb.Tournament) {
	t.Helper()
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/tournaments", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httpReq)
	if w.Code != http.StatusCreated {
		return w, nil
	}
	var tournament pb.Tournament
	if err := proto.Unmarshal(w.Body.Bytes(), &tournament); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return w, &tournament
}

// TestCreateTournamentDerivesSlugFromName covers the admin UI path:
// the create form leaves the slug empty, and the API used to answer
// 400, so a tournament could not be created from the dashboard.
func TestCreateTournamentDerivesSlugFromName(t *testing.T) {
	r := newPageTestRouter(t, 0)

	w, tournament := postTournament(t, r, &pb.TournamentRequest{Name: "UI Test Tournament"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want %d (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if tournament.GetSlug() != "ui-test-tournament" {
		t.Fatalf("slug = %q, want %q", tournament.GetSlug(), "ui-test-tournament")
	}
}

// TestCreateTournamentKeepsAnExplicitSlug makes sure the derivation
// only fills a blank slug and never overrides the operator's own.
func TestCreateTournamentKeepsAnExplicitSlug(t *testing.T) {
	r := newPageTestRouter(t, 0)

	w, tournament := postTournament(t, r, &pb.TournamentRequest{Name: "UI Test Tournament", Slug: "custom-slug"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want %d (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if tournament.GetSlug() != "custom-slug" {
		t.Fatalf("slug = %q, want %q", tournament.GetSlug(), "custom-slug")
	}
}

// TestCreateTournamentRejectsAnUnslugableName documents the remaining
// case: a name whose scraper-style derivation is not slug-shaped has
// to be rejected, because inventing a different derivation would make
// the admin row and the scraper row disagree about the slug.
func TestCreateTournamentRejectsAnUnslugableName(t *testing.T) {
	r := newPageTestRouter(t, 0)

	w, _ := postTournament(t, r, &pb.TournamentRequest{Name: "B. League"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
