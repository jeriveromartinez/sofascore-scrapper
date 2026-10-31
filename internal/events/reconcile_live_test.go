package events

import (
	"context"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
)

func TestCloseStaleLive_FinishesEventsThatLeftTheFeed(t *testing.T) {
	repo, db := setupCurrentEventsDB(t)

	league := tournaments.Tournament{Name: "League", Slug: "league"}
	mustCreate(t, db, &league)
	mustCreate(t, db, &tournaments.GlobalTournamentConfig{TournamentID: league.ID})

	now := time.Now()
	zombie := seedCurrentEvent(t, db, "zombie", "inprogress", now.Add(-48*time.Hour).UnixMilli(), league.ID)
	refreshed := seedCurrentEvent(t, db, "refreshed", "inprogress", now.Add(-48*time.Hour).UnixMilli(), league.ID)
	justStarted := seedCurrentEvent(t, db, "just-started", "inprogress", now.Add(-time.Hour).UnixMilli(), league.ID)
	done := seedCurrentEvent(t, db, "done", "finished", now.Add(-48*time.Hour).UnixMilli(), league.ID)

	age := func(id uint, d time.Duration) {
		t.Helper()
		if err := db.Model(&Event{}).Where("id = ?", id).
			Update("scraped_at", now.Add(-d).Unix()).Error; err != nil {
			t.Fatalf("age event %d: %v", id, err)
		}
	}
	age(zombie, 2*time.Hour)
	age(refreshed, time.Minute)
	age(justStarted, 2*time.Hour)

	closed, err := repo.CloseStaleLive(context.Background(), now)
	if err != nil {
		t.Fatalf("CloseStaleLive: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed %d events, want 1", closed)
	}

	status := func(id uint) string {
		t.Helper()
		var event Event
		if err := db.First(&event, id).Error; err != nil {
			t.Fatalf("load event %d: %v", id, err)
		}
		return event.StatusType
	}
	if got := status(zombie); got != "finished" {
		t.Errorf("zombie status = %q, want finished", got)
	}
	if got := status(refreshed); got != "inprogress" {
		t.Errorf("refreshed status = %q, want inprogress", got)
	}
	if got := status(justStarted); got != "inprogress" {
		t.Errorf("just-started status = %q, want inprogress", got)
	}
	if got := status(done); got != "finished" {
		t.Errorf("done status = %q, want finished", got)
	}
}
