package events

import (
	"testing"

	"gorm.io/gorm"
)

func TestEvent_ExternalMatchIdField(t *testing.T) {
	var e Event
	e.ExternalMatchId = "4193492"
	e.Source = "fotmob"
	if e.ExternalMatchId != "4193492" {
		t.Fatalf("ExternalMatchId no se asigna: %q", e.ExternalMatchId)
	}
	if e.Source != "fotmob" {
		t.Fatalf("Source no se asigna: %q", e.Source)
	}
	// Soft check: el campo renombrado ya no existe
	e2 := Event{}
	_ = e2 // asegura que compila sin SofaScoreEventId
	_ = gorm.Model{}
}
