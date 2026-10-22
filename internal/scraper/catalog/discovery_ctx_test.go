package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jeriveromartinez/scores365-scrapper/internal/scores365"
	"gorm.io/gorm"
)

func TestDiscovery_CancelledDuringPersistReturnsError(t *testing.T) {
	srv, _ := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	db := newDiscoveryDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := db.Callback().Query().Before("gorm:query").Register("test:cancel_run", func(*gorm.DB) { cancel() }); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})

	res, err := DiscoverySports(ctx, NewRepository(db), client, []string{"tennis", "basketball"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DiscoverySports error = %v (result %+v), want context.Canceled", err, res)
	}
	if res.Skipped != 0 {
		t.Fatalf("cancellation counted as %d skipped leagues", res.Skipped)
	}
}

func TestDiscovery_PersistPhaseIsBounded(t *testing.T) {
	srv, _ := newCompetitionsServer(t, competitionsBody)
	defer srv.Close()

	// A negative budget makes the persist context already expired when
	// it is created, so the deadline is observed deterministically.
	// A 1ns budget instead races the context's timer against the
	// driver's own cancellation check, and the sweep sometimes
	// finishes before the timer fires.
	previous := discoveryPersistTimeout
	discoveryPersistTimeout = -time.Second
	t.Cleanup(func() { discoveryPersistTimeout = previous })

	client := scores365.NewClient(scores365.Options{BaseURL: srv.URL})
	_, err := DiscoverySports(context.Background(), NewRepository(newDiscoveryDB(t)), client, []string{"tennis"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DiscoverySports error = %v, want context.DeadlineExceeded", err)
	}
}
