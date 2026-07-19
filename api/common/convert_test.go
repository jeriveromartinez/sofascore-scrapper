package common

import (
	"testing"
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/models"
)

func TestUserToProto(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	u := models.User{Email: "test@example.com"}
	u.ID = 1
	u.CreatedAt = now
	u.UpdatedAt = now

	result := UserToProto(u)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Id != 1 {
		t.Errorf("expected Id=1, got %d", result.Id)
	}
	if result.Email != "test@example.com" {
		t.Errorf("expected Email='test@example.com', got %s", result.Email)
	}
	if result.CreatedAt != FormatTime(now) {
		t.Errorf("expected CreatedAt='%s', got '%s'", FormatTime(now), result.CreatedAt)
	}
}

func TestUsersToProto(t *testing.T) {
	users := []models.User{{Email: "a@b.com"}, {Email: "c@d.com"}}
	users[0].ID = 1
	users[1].ID = 2

	result := UsersToProto(users)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].Email != "a@b.com" {
		t.Errorf("expected first email='a@b.com', got %s", result[0].Email)
	}
	if result[1].Email != "c@d.com" {
		t.Errorf("expected second email='c@d.com', got %s", result[1].Email)
	}
}

func TestUsersToProtoEmpty(t *testing.T) {
	result := UsersToProto(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestDomainToProto(t *testing.T) {
	d := models.Domain{Domain: "example.com", UserID: 2}
	d.ID = 1

	result := DomainToProto(d)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Domain != "example.com" {
		t.Errorf("expected Domain='example.com', got %s", result.Domain)
	}
	if result.UserId != 2 {
		t.Errorf("expected UserId=2, got %d", result.UserId)
	}
}

func TestDomainToProtoWithUser(t *testing.T) {
	user := &models.User{Email: "owner@test.com"}
	user.ID = 5
	d := models.Domain{Domain: "test.com", UserID: 5, User: user}
	d.ID = 10

	result := DomainToProto(d)
	if result.User == nil {
		t.Fatal("expected non-nil User")
	}
	if result.User.Email != "owner@test.com" {
		t.Errorf("expected User.Email='owner@test.com', got %s", result.User.Email)
	}
}

func TestDomainsToProto(t *testing.T) {
	domains := []models.Domain{{Domain: "a.com"}, {Domain: "b.com"}}
	domains[0].ID = 1
	domains[1].ID = 2

	result := DomainsToProto(domains)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestDeviceToProto(t *testing.T) {
	d := models.Device{Token: "tok123", Platform: "android", Name: "MyDevice", Version: "2.0", LastSeen: 1700000000}
	d.ID = 1

	result := DeviceToProto(d)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Token != "tok123" {
		t.Errorf("expected Token='tok123', got %s", result.Token)
	}
	if result.Platform != "android" {
		t.Errorf("expected Platform='android', got %s", result.Platform)
	}
	if result.LastSeen != 1700000000 {
		t.Errorf("expected LastSeen=1700000000, got %d", result.LastSeen)
	}
}

func TestDevicesToProto(t *testing.T) {
	devices := []models.Device{{Token: "a"}, {Token: "b"}}
	result := DevicesToProto(devices)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestTournamentToProto(t *testing.T) {
	tm := models.Tournament{Name: "LaLiga", Slug: "laliga", Region: "Spain"}
	tm.ID = 1

	result := TournamentToProto(tm)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Name != "LaLiga" {
		t.Errorf("expected Name='LaLiga', got %s", result.Name)
	}
	if result.Slug != "laliga" {
		t.Errorf("expected Slug='laliga', got %s", result.Slug)
	}
}

func TestTournamentPtrToProtoNil(t *testing.T) {
	result := TournamentPtrToProto(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestTournamentPtrToProto(t *testing.T) {
	tm := &models.Tournament{Name: "EPL", Slug: "epl", Region: "England"}
	tm.ID = 2

	result := TournamentPtrToProto(tm)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Name != "EPL" {
		t.Errorf("expected Name='EPL', got %s", result.Name)
	}
}

func TestTournamentsToProto(t *testing.T) {
	ts := []models.Tournament{{Name: "A"}, {Name: "B"}}
	result := TournamentsToProto(ts)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestTeamPtrToProtoNil(t *testing.T) {
	result := TeamPtrToProto(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestTeamPtrToProto(t *testing.T) {
	tm := &models.Team{TeamId: 42, Name: "FC Test", LogoUrl: "/logo.png", PrimaryColor: "#ff0000", SecondaryColor: "#00ff00", TextColor: "#ffffff"}
	result := TeamPtrToProto(tm)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.TeamId != 42 {
		t.Errorf("expected TeamId=42, got %d", result.TeamId)
	}
	if result.Name != "FC Test" {
		t.Errorf("expected Name='FC Test', got %s", result.Name)
	}
}

func TestEventToProto(t *testing.T) {
	homeTeam := &models.Team{TeamId: 1, Name: "Home"}
	awayTeam := &models.Team{TeamId: 2, Name: "Away"}
	league := &models.Tournament{Name: "Champions League"}
	league.ID = 3

	e := models.SofaScoreEvent{
		SofaScoreEventId:            100,
		Sport:                       "football",
		HomeScore:                   2,
		HomeTeamId:                  1,
		AwayScore:                   1,
		AwayTeamId:                  2,
		ScrapedAt:                   1700000000,
		StartTimestamp:              1699999900,
		CurrentPeriodStartTimestamp: 1700000100,
		Slug:                        "home-vs-away",
		HomeTeamModel:               homeTeam,
		AwayTeamModel:               awayTeam,
		League:                      league,
		LeagueId:                    3,
	}
	e.ID = 50

	result := EventToProto(e)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Sport != "football" {
		t.Errorf("expected Sport='football', got %s", result.Sport)
	}
	if result.HomeScore != 2 {
		t.Errorf("expected HomeScore=2, got %d", result.HomeScore)
	}
	if result.AwayScore != 1 {
		t.Errorf("expected AwayScore=1, got %d", result.AwayScore)
	}
	if result.TeamHome == nil {
		t.Error("expected non-nil TeamHome")
	}
	if result.TeamAway == nil {
		t.Error("expected non-nil TeamAway")
	}
	if result.League == nil {
		t.Error("expected non-nil League")
	}
	if result.Id != 50 {
		t.Errorf("expected Id=50, got %d", result.Id)
	}
}

func TestEventsToProto(t *testing.T) {
	home := &models.Team{TeamId: 10, Name: "H", LogoUrl: "/team/10/image"}
	away := &models.Team{TeamId: 20, Name: "A", LogoUrl: "/team/20/image"}
	events := []models.SofaScoreEvent{
		{SofaScoreEventId: 1, HomeTeamModel: home, AwayTeamModel: away},
		{SofaScoreEventId: 2, HomeTeamModel: home, AwayTeamModel: away},
	}

	result := EventsToProto(events)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].TeamHome.LogoUrl != "/api/app/v1/team/10/image" {
		t.Errorf("expected LogoUrl='/api/app/v1/team/10/image', got %s", result[0].TeamHome.LogoUrl)
	}
}

func TestPlaybackToProtoNil(t *testing.T) {
	result := PlaybackToProto(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestPlaybackToProto(t *testing.T) {
	p := &models.PlaybackLog{DeviceID: 5, Content: "test-stream", StartedAt: 1700000000, EndedAt: 1700003600}
	p.ID = 99

	result := PlaybackToProto(p)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.DeviceId != 5 {
		t.Errorf("expected DeviceId=5, got %d", result.DeviceId)
	}
	if result.Content != "test-stream" {
		t.Errorf("expected Content='test-stream', got %s", result.Content)
	}
	if result.Id != 99 {
		t.Errorf("expected Id=99, got %d", result.Id)
	}
}

func TestPlaybackListToProto(t *testing.T) {
	pl := []*models.PlaybackLog{
		{DeviceID: 1, Content: "a"},
		{DeviceID: 2, Content: "b"},
	}
	pl[0].ID = 1
	pl[1].ID = 2

	result := PlaybackListToProto(pl, 100)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Total != 100 {
		t.Errorf("expected Total=100, got %d", result.Total)
	}
	if len(result.List) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.List))
	}
}

func TestPlaybackListToProtoEmpty(t *testing.T) {
	result := PlaybackListToProto(nil, 0)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Total != 0 {
		t.Errorf("expected Total=0, got %d", result.Total)
	}
	if len(result.List) != 0 {
		t.Errorf("expected empty, got %d", len(result.List))
	}
}

func TestGlobalConfigToProto(t *testing.T) {
	tm := &models.Tournament{Name: "WC", Slug: "wc", Region: "World"}
	tm.ID = 1
	g := models.GlobalTournamentConfig{TournamentID: 1, Tournament: tm}
	g.ID = 10

	result := GlobalConfigToProto(g)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.TournamentId != 1 {
		t.Errorf("expected TournamentId=1, got %d", result.TournamentId)
	}
	if result.Tournament.Name != "WC" {
		t.Errorf("expected Tournament.Name='WC', got %s", result.Tournament.Name)
	}
	if result.Id != 10 {
		t.Errorf("expected Id=10, got %d", result.Id)
	}
}

func TestGlobalConfigsToProto(t *testing.T) {
	gs := []models.GlobalTournamentConfig{{TournamentID: 1}, {TournamentID: 2}}
	result := GlobalConfigsToProto(gs)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestDeviceTournamentToProto(t *testing.T) {
	tm := &models.Tournament{Name: "LaLiga"}
	tm.ID = 5
	dev := &models.Device{Token: "dtok"}
	dev.ID = 3
	dt := models.DeviceTournament{DeviceID: 3, TournamentID: 5, Device: dev, Tournament: tm}
	dt.ID = 7

	result := DeviceTournamentToProto(dt)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.DeviceId != 3 {
		t.Errorf("expected DeviceId=3, got %d", result.DeviceId)
	}
	if result.TournamentId != 5 {
		t.Errorf("expected TournamentId=5, got %d", result.TournamentId)
	}
	if result.Device.Token != "dtok" {
		t.Errorf("expected Device.Token='dtok', got %s", result.Device.Token)
	}
	if result.Tournament.Name != "LaLiga" {
		t.Errorf("expected Tournament.Name='LaLiga', got %s", result.Tournament.Name)
	}
}

func TestDeviceTournamentsToProto(t *testing.T) {
	dts := []models.DeviceTournament{{DeviceID: 1}, {DeviceID: 2}}
	result := DeviceTournamentsToProto(dts)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestApkToProto(t *testing.T) {
	v := models.ApkVersion{
		Version:          "2.0",
		FileName:         "app.apk",
		FileSize:         5000000,
		Description:      "Update",
		IsActive:         true,
		PackageName:      "com.test.app",
		VersionCode:      10,
		MinSDKVersion:    21,
		TargetSDKVersion: 33,
		DownloadToken:    "uuid-token",
		TotalDownloads:   100,
		IPTVUrl:          "http://example.com",
	}
	v.ID = 10

	result := ApkToProto(v, "http://download.url/apk")
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.Version != "2.0" {
		t.Errorf("expected Version='2.0', got %s", result.Version)
	}
	if result.DownloadUrl != "http://download.url/apk" {
		t.Errorf("expected DownloadUrl='http://download.url/apk', got %s", result.DownloadUrl)
	}
	if result.PackageName != "com.test.app" {
		t.Errorf("expected PackageName='com.test.app', got %s", result.PackageName)
	}
	if result.Id != 10 {
		t.Errorf("expected Id=10, got %d", result.Id)
	}
	if result.PanelUrl != "http://example.com" {
		t.Errorf("expected PanelUrl='http://example.com', got %s", result.PanelUrl)
	}
}

func TestApksToProto(t *testing.T) {
	versions := []models.ApkVersion{
		{DownloadToken: "tok-a"},
		{DownloadToken: "tok-b"},
	}
	result := ApksToProto(versions)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].DownloadUrl != "/api/app/v1/apk/download/tok-a" {
		t.Errorf("expected DownloadUrl='/api/app/v1/apk/download/tok-a', got %s", result[0].DownloadUrl)
	}
	if result[1].DownloadUrl != "/api/app/v1/apk/download/tok-b" {
		t.Errorf("expected DownloadUrl='/api/app/v1/apk/download/tok-b', got %s", result[1].DownloadUrl)
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{"zero time", time.Time{}, ""},
		{"non-zero time", time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC), "2024-06-15T10:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTime(tt.input)
			if got != tt.expected {
				t.Errorf("FormatTime(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
