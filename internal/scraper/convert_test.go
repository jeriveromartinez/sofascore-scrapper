package scraper

import (
	"testing"
)

func TestToEvent_TimestampConversionToMs(t *testing.T) {
	source := APIEvent{
		ID:             123456,
		Slug:           "test-match",
		StartTimestamp: 1710000000,
		Status: struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
			Type        string `json:"type"`
		}{Type: "inprogress"},
	}
	source.Time.CurrentPeriodStartTimestamp = 1710000000
	source.HomeTeam = TeamApi{ID: 10, Name: "Home"}
	source.AwayTeam = TeamApi{ID: 20, Name: "Away"}
	source.Tournament.UniqueTournament.ID = 1
	source.Tournament.UniqueTournament.Name = "Test League"
	source.Tournament.UniqueTournament.Slug = "test-league"
	source.Tournament.UniqueTournament.Category.Name = "category"
	source.Tournament.UniqueTournament.Category.Slug = "category-slug"

	event := ToEvent(source, "football")

	if event.StartTimestamp != 1710000000000 {
		t.Errorf("StartTimestamp: want 1710000000000, got %d", event.StartTimestamp)
	}
	if event.CurrentPeriodStartTimestamp != 1710000000000 {
		t.Errorf("CurrentPeriodStartTimestamp: want 1710000000000, got %d", event.CurrentPeriodStartTimestamp)
	}
	if event.StatusType != "inprogress" {
		t.Errorf("StatusType: want inprogress, got %s", event.StatusType)
	}
}

func TestToEvent_ZeroTimestampsGuarded(t *testing.T) {
	source := APIEvent{
		ID:             1,
		Slug:           "zero-test",
		StartTimestamp: 0,
		Status: struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
			Type        string `json:"type"`
		}{Type: "notstarted"},
	}
	source.Time.CurrentPeriodStartTimestamp = 0
	source.HomeTeam = TeamApi{ID: 1, Name: "H"}
	source.AwayTeam = TeamApi{ID: 2, Name: "A"}
	source.Tournament.UniqueTournament.ID = 1
	source.Tournament.UniqueTournament.Name = "L"
	source.Tournament.UniqueTournament.Slug = "l"
	source.Tournament.UniqueTournament.Category.Name = "c"
	source.Tournament.UniqueTournament.Category.Slug = "c"

	event := ToEvent(source, "football")

	if event.StartTimestamp != 0 {
		t.Errorf("StartTimestamp: want 0, got %d", event.StartTimestamp)
	}
	if event.CurrentPeriodStartTimestamp != 0 {
		t.Errorf("CurrentPeriodStartTimestamp: want 0, got %d", event.CurrentPeriodStartTimestamp)
	}
}

func TestToEvent_OverflowGuarded(t *testing.T) {
	source := APIEvent{
		ID:             1,
		Slug:           "overflow-test",
		StartTimestamp: 1<<62 + 1,
		Status: struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
			Type        string `json:"type"`
		}{Type: "finished"},
	}
	source.Time.CurrentPeriodStartTimestamp = 1 << 62
	source.HomeTeam = TeamApi{ID: 1, Name: "H"}
	source.AwayTeam = TeamApi{ID: 2, Name: "A"}
	source.Tournament.UniqueTournament.ID = 1
	source.Tournament.UniqueTournament.Name = "L"
	source.Tournament.UniqueTournament.Slug = "l"
	source.Tournament.UniqueTournament.Category.Name = "c"
	source.Tournament.UniqueTournament.Category.Slug = "c"

	event := ToEvent(source, "football")

	if event.StartTimestamp != 0 {
		t.Errorf("StartTimestamp: want 0 (overflow guarded), got %d", event.StartTimestamp)
	}
	if event.CurrentPeriodStartTimestamp != 0 {
		t.Errorf("CurrentPeriodStartTimestamp: want 0 (overflow guarded), got %d", event.CurrentPeriodStartTimestamp)
	}
}

func TestToEvent_StatusTypeMapping(t *testing.T) {
	tests := []struct {
		name       string
		statusType string
	}{
		{"inprogress", "inprogress"},
		{"notstarted", "notstarted"},
		{"finished", "finished"},
		{"postponed", "postponed"},
		{"canceled", "canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := APIEvent{
				ID:             1,
				Slug:           tt.name,
				StartTimestamp: 1,
				Status: struct {
					Code        int    `json:"code"`
					Description string `json:"description"`
					Type        string `json:"type"`
				}{Type: tt.statusType},
			}
			source.Time.CurrentPeriodStartTimestamp = 1
			source.HomeTeam = TeamApi{ID: 1, Name: "H"}
			source.AwayTeam = TeamApi{ID: 2, Name: "A"}
			source.Tournament.UniqueTournament.ID = 1
			source.Tournament.UniqueTournament.Name = "L"
			source.Tournament.UniqueTournament.Slug = "l"
			source.Tournament.UniqueTournament.Category.Name = "c"
			source.Tournament.UniqueTournament.Category.Slug = "c"

			event := ToEvent(source, "football")
			if event.StatusType != tt.statusType {
				t.Errorf("StatusType: want %s, got %s", tt.statusType, event.StatusType)
			}
		})
	}
}
