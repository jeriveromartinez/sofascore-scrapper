package events

import (
	"strconv"
	"testing"

	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
)

// TestEventToExternalProto_PopulatesDeprecatedSofaScoreId covers the
// backwards-compat shim added in fix A2 (PR #122): old clients built
// against the pre-FotMob wire schema read tag 4 as int64. When the
// stored ExternalMatchId is numeric, EventToExternalProto must echo
// the value into the new SofaScoreEventIdDeprecated field so those
// clients keep seeing the legacy id. Non-numeric ids (e.g. "4193492"
// as FotMob emits) leave the deprecated field at zero.
func TestEventToExternalProto_PopulatesDeprecatedSofaScoreId(t *testing.T) {
	numeric := "4193492"
	e := Event{ExternalMatchId: numeric}
	got := EventToExternalProto(e)
	if got.ExternalMatchId != numeric {
		t.Fatalf("ExternalMatchId: want %q, got %q", numeric, got.ExternalMatchId)
	}
	want, _ := strconv.ParseInt(numeric, 10, 64)
	if got.SofaScoreEventIdDeprecated != want {
		t.Errorf("SofaScoreEventIdDeprecated: want %d, got %d", want, got.SofaScoreEventIdDeprecated)
	}
}

// TestEventToExternalProto_LeavesDeprecatedZeroForNonNumeric covers the
// non-numeric FotMob-id case: we cannot stuff a string into an int64,
// so the deprecated field stays at the proto zero value and the new
// ExternalMatchId carries the value.
func TestEventToExternalProto_LeavesDeprecatedZeroForNonNumeric(t *testing.T) {
	e := Event{ExternalMatchId: "fotmob-abc-42"}
	got := EventToExternalProto(e)
	if got.ExternalMatchId != "fotmob-abc-42" {
		t.Fatalf("ExternalMatchId: want %q, got %q", "fotmob-abc-42", got.ExternalMatchId)
	}
	if got.SofaScoreEventIdDeprecated != 0 {
		t.Errorf("SofaScoreEventIdDeprecated: want 0 for non-numeric id, got %d", got.SofaScoreEventIdDeprecated)
	}
}

// TestEventToExternalProto_DeprecatedFieldIsExposed ensures the
// generated Go binding still exposes the deprecated int64 accessor
// after the proto regeneration (i.e. we did not accidentally drop the
// field by re-running protoc).
func TestEventToExternalProto_DeprecatedFieldIsExposed(t *testing.T) {
	msg := &pb.ExternalEvent{SofaScoreEventIdDeprecated: 12345}
	if msg.GetSofaScoreEventIdDeprecated() != 12345 {
		t.Fatalf("GetSofaScoreEventIdDeprecated: want 12345, got %d", msg.GetSofaScoreEventIdDeprecated())
	}
}
