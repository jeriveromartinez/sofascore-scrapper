package characterization

import (
	"sync"
	"testing"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm/schema"
)

func hasUniqueIndex(s *schema.Schema, fieldName string) bool {
	for _, idx := range s.ParseIndexes() {
		if idx.Class == "UNIQUE" {
			for _, f := range idx.Fields {
				if f.Field != nil && f.Field.Name == fieldName {
					return true
				}
			}
		}
	}
	return false
}

func TestSchemaUser(t *testing.T) {
	s, err := schema.Parse(&users.User{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "users" {
		t.Errorf("expected table 'users', got %q", s.Table)
	}

	email := s.LookUpField("Email")
	if email == nil {
		t.Error("expected field 'Email' in User")
	} else {
		if !email.NotNull {
			t.Error("Email: expected NotNull")
		}
		if !hasUniqueIndex(s, "Email") {
			t.Error("Email: expected unique index")
		}
	}
	password := s.LookUpField("Password")
	if password == nil {
		t.Error("expected field 'Password' in User")
	} else if !password.NotNull {
		t.Error("Password: expected NotNull")
	}
}

func TestSchemaTournament(t *testing.T) {
	s, err := schema.Parse(&tournaments.Tournament{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "tournaments" {
		t.Errorf("expected table 'tournaments', got %q", s.Table)
	}
	for _, name := range []string{"Name", "Slug", "Region"} {
		field := s.LookUpField(name)
		if field == nil {
			t.Errorf("expected field %q in Tournament", name)
		}
	}
}

func TestSchemaTeam(t *testing.T) {
	s, err := schema.Parse(&events.Team{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "teams" {
		t.Errorf("expected table 'teams', got %q", s.Table)
	}
	teamID := s.LookUpField("TeamId")
	if teamID == nil {
		t.Error("expected field 'TeamId' in Team")
	} else if !hasUniqueIndex(s, "TeamId") {
		t.Error("TeamId should have unique index")
	}
}

func TestSchemaDevice(t *testing.T) {
	s, err := schema.Parse(&devices.Device{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "devices" {
		t.Errorf("expected table 'devices', got %q", s.Table)
	}
	token := s.LookUpField("Token")
	if token == nil {
		t.Error("expected field 'Token' in Device")
	} else {
		if !token.NotNull {
			t.Error("Token should be NotNull")
		}
		if !hasUniqueIndex(s, "Token") {
			t.Error("Token should have unique index")
		}
	}
}

func TestSchemaPlaybackLog(t *testing.T) {
	s, err := schema.Parse(&playback.PlaybackLog{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "playback_logs" {
		t.Errorf("expected table 'playback_logs', got %q", s.Table)
	}
	deviceID := s.LookUpField("DeviceID")
	if deviceID == nil {
		t.Error("expected field 'DeviceID' in PlaybackLog")
	} else if !deviceID.NotNull {
		t.Error("DeviceID should be NotNull")
	}
	content := s.LookUpField("Content")
	if content == nil {
		t.Error("expected field 'Content' in PlaybackLog")
	} else if !content.NotNull {
		t.Error("Content should be NotNull")
	}
}

func TestSchemaApkVersion(t *testing.T) {
	s, err := schema.Parse(&apk.ApkVersion{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "apk_versions" {
		t.Errorf("expected table 'apk_versions', got %q", s.Table)
	}
	version := s.LookUpField("Version")
	if version == nil {
		t.Error("expected field 'Version' in ApkVersion")
	} else if !version.NotNull {
		t.Error("Version should be NotNull")
	}
	packageName := s.LookUpField("PackageName")
	if packageName == nil {
		t.Error("expected field 'PackageName' in ApkVersion")
	} else if !packageName.NotNull {
		t.Error("PackageName should be NotNull")
	}
	downloadToken := s.LookUpField("DownloadToken")
	if downloadToken == nil {
		t.Error("expected field 'DownloadToken' in ApkVersion")
	} else if !hasUniqueIndex(s, "DownloadToken") {
		t.Error("DownloadToken should have unique index")
	}
	isActive := s.LookUpField("IsActive")
	if isActive == nil {
		t.Error("expected field 'IsActive' in ApkVersion")
	} else if isActive.DefaultValue != "true" {
		t.Errorf("expected IsActive default 'true', got %q", isActive.DefaultValue)
	}
	iptvURL := s.LookUpField("IPTVUrl")
	if iptvURL == nil {
		t.Error("expected field 'IPTVUrl' in ApkVersion")
	} else if iptvURL.DefaultValue != "http://5.mdtv.me" {
		t.Errorf("expected IPTVUrl default 'http://5.mdtv.me', got %q", iptvURL.DefaultValue)
	}
}

func TestSchemaDomain(t *testing.T) {
	s, err := schema.Parse(&domains.Domain{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "domains" {
		t.Errorf("expected table 'domains', got %q", s.Table)
	}
	domain := s.LookUpField("Domain")
	if domain == nil {
		t.Error("expected field 'Domain' in Domain")
	} else {
		if !domain.NotNull {
			t.Error("Domain should be NotNull")
		}
		if !hasUniqueIndex(s, "Domain") {
			t.Error("Domain should have unique index")
		}
	}
	userID := s.LookUpField("UserID")
	if userID == nil {
		t.Error("expected field 'UserID' in Domain")
	} else if !userID.NotNull {
		t.Error("UserID should be NotNull")
	}
}

func TestSchemaSofaScoreEvent(t *testing.T) {
	s, err := schema.Parse(&events.Event{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "events" {
		t.Errorf("expected table 'events', got %q", s.Table)
	}
	eventID := s.LookUpField("SofaScoreEventId")
	if eventID == nil {
		t.Error("expected field 'SofaScoreEventId' in Event")
	} else if !hasUniqueIndex(s, "SofaScoreEventId") {
		t.Error("SofaScoreEventId should have unique index")
	}
}

func TestSchemaDeviceTournament(t *testing.T) {
	s, err := schema.Parse(&tournaments.DeviceTournament{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "device_tournaments" {
		t.Errorf("expected table 'device_tournaments', got %q", s.Table)
	}
	for _, name := range []string{"DeviceID", "TournamentID"} {
		field := s.LookUpField(name)
		if field == nil {
			t.Errorf("expected field %q in DeviceTournament", name)
		} else if !field.NotNull {
			t.Errorf("%s should be NotNull", name)
		}
	}
}

func TestSchemaGlobalTournamentConfig(t *testing.T) {
	s, err := schema.Parse(&tournaments.GlobalTournamentConfig{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "global_tournament_configs" {
		t.Errorf("expected table 'global_tournament_configs', got %q", s.Table)
	}
	tournamentID := s.LookUpField("TournamentID")
	if tournamentID == nil {
		t.Error("expected field 'TournamentID' in GlobalTournamentConfig")
	} else {
		if !tournamentID.NotNull {
			t.Error("TournamentID should be NotNull")
		}
		if !hasUniqueIndex(s, "TournamentID") {
			t.Error("TournamentID should have unique index")
		}
	}
}

func TestSchemaRefreshToken(t *testing.T) {
	s, err := schema.Parse(&auth.RefreshToken{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "refresh_tokens" {
		t.Errorf("expected table 'refresh_tokens', got %q", s.Table)
	}
	userID := s.LookUpField("UserID")
	if userID == nil {
		t.Error("expected field 'UserID' in RefreshToken")
	} else if !userID.NotNull {
		t.Error("UserID should be NotNull")
	}
	tokenID := s.LookUpField("TokenID")
	if tokenID == nil {
		t.Error("expected field 'TokenID' in RefreshToken")
	} else {
		if !tokenID.NotNull {
			t.Error("TokenID should be NotNull")
		}
		if !hasUniqueIndex(s, "TokenID") {
			t.Error("TokenID should have unique index")
		}
	}
}

func TestSchemaContentStat(t *testing.T) {
	s, err := schema.Parse(&reporting.ContentStat{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "content_stats" {
		t.Errorf("expected table 'content_stats', got %q", s.Table)
	}
	for _, name := range []string{"ContentHash", "PeriodType", "PeriodStart", "Seconds", "Views"} {
		field := s.LookUpField(name)
		if field == nil {
			t.Errorf("expected field %q in ContentStat", name)
		} else if !field.NotNull {
			t.Errorf("%s should be NotNull", name)
		}
	}
}

func TestSchemaCrashReport(t *testing.T) {
	s, err := schema.Parse(&reporting.CrashReport{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse failed: %v", err)
	}
	if s.Table != "crash_reports" {
		t.Errorf("expected table 'crash_reports', got %q", s.Table)
	}
	for _, name := range []string{"Fatal", "Error", "StackTrace", "Context"} {
		field := s.LookUpField(name)
		if field == nil {
			t.Errorf("expected field %q in CrashReport", name)
		}
	}
}
