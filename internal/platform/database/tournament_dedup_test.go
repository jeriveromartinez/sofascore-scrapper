package database

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jeriveromartinez/scores365-scrapper/internal/devices"
	"github.com/jeriveromartinez/scores365-scrapper/internal/domains"
	"github.com/jeriveromartinez/scores365-scrapper/internal/events"
	"github.com/jeriveromartinez/scores365-scrapper/internal/scraper/catalog"
	"github.com/jeriveromartinez/scores365-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	"gorm.io/gorm"
)

// tournamentDedupDBCounter hands out the name of the in-memory database
// each call gets. A shared-cache database survives until its last connection
// closes, so the name has to be unique per call.
var tournamentDedupDBCounter atomic.Int64

// openDedupTestDB opens a sqlite-backed database with every table the
// FotMob tournament migration touches.
func openDedupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:tournament-dedup-%d?mode=memory&cache=shared", tournamentDedupDBCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The pool is closed so the named database does not outlive the test.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&users.User{},
		&domains.Domain{},
		&devices.Device{},
		&tournaments.Tournament{},
		&tournaments.DeviceTournament{},
		&tournaments.GlobalTournamentConfig{},
		&events.Team{},
		&events.Event{},
		&catalog.ScraperLeague{},
	); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	return db
}

// TestOpenDedupTestDBUsesAFreshDatabasePerCall pins the hermeticity of the
// in-memory database: a shared-cache database survives until its last
// connection closes, so a second call must not attach to the database the
// first one seeded.
func TestOpenDedupTestDBUsesAFreshDatabasePerCall(t *testing.T) {
	first := openDedupTestDB(t)
	seedTournament(t, first, 47, "Premier League", "premier-league", "GB")

	second := openDedupTestDB(t)
	seedTournament(t, second, 47, "Premier League", "premier-league", "GB")
}

func seedTournament(t *testing.T, db *gorm.DB, id uint, name, slug, region string) {
	t.Helper()
	row := tournaments.Tournament{
		Model:  gorm.Model{ID: id},
		Name:   name,
		Slug:   slug,
		Region: region,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed tournament %d: %v", id, err)
	}
}

// liveTournamentIDs returns the ids of the tournaments the admin list
// shows, i.e. the rows that are not soft-deleted.
func liveTournamentIDs(t *testing.T, db *gorm.DB) []uint {
	t.Helper()
	var rows []tournaments.Tournament
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read tournaments: %v", err)
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func containsID(ids []uint, want uint) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestMigrateFotMobTournamentDuplicates_ResolvesDuplicates is the
// regression test for the duplicated tournament rows the FotMob id
// shift left behind: every league the pre-shift code path wrote exists
// twice, once under the raw FotMob id and once under the shifted id.
//
// The legacy row is the one the admin list shows as a duplicate, and
// the references that still point at it (events, device assignments,
// global configs) have to follow the league to the shifted row.
func TestMigrateFotMobTournamentDuplicates_ResolvesDuplicates(t *testing.T) {
	db := openDedupTestDB(t)

	const (
		legacyPremierLeague  = 47
		legacyLaLiga         = 87
		legacyEFL            = 938218
		legacyAdminCollision = 100
	)
	shiftedPremierLeague := uint(uint64(legacyPremierLeague) + tournaments.FotMobTournamentIDBase)
	shiftedLaLiga := uint(uint64(legacyLaLiga) + tournaments.FotMobTournamentIDBase)
	shiftedAdminCollision := uint(uint64(legacyAdminCollision) + tournaments.FotMobTournamentIDBase)
	const (
		adminTournament = 1_000_000_001
		scores365League = 6_000_000_047
	)

	seedTournament(t, db, legacyPremierLeague, "Premier League", "premier-league", "GB")
	seedTournament(t, db, shiftedPremierLeague, "Premier League", "premier-league", "GB")
	seedTournament(t, db, legacyLaLiga, "LaLiga", "laliga", "ES")
	seedTournament(t, db, shiftedLaLiga, "LaLiga", "laliga", "ES")
	// A legacy row without a shifted twin: the league was never
	// scraped after the shift, so the row is the only one there is.
	seedTournament(t, db, legacyEFL, "EFL Championship", "efl-championship", "GB")
	// An admin tournament that landed on an upstream id: its name
	// matches neither the shifted twin nor the catalog, so both rows
	// have to survive.
	seedTournament(t, db, legacyAdminCollision, "Admin Cup", "admin-cup", "")
	seedTournament(t, db, shiftedAdminCollision, "Serie B", "serie-b", "IT")
	// Rows outside the legacy band: the admin band and the 365scores
	// band were never shifted.
	seedTournament(t, db, adminTournament, "Admin Band", "admin-band", "")
	seedTournament(t, db, scores365League, "365scores League", "365scores-league", "AR")

	if err := db.Create(&catalog.ScraperLeague{
		Source:         "fotmob",
		SourceLeagueId: "47",
		Name:           "Premier League",
		Country:        "GB",
		Sport:          "football",
		Enabled:        true,
	}).Error; err != nil {
		t.Fatalf("seed scraper league: %v", err)
	}

	device := devices.Device{Token: "dedup-token"}
	if err := db.Create(&device).Error; err != nil {
		t.Fatalf("seed device: %v", err)
	}

	team := events.Team{TeamId: 10, Name: "Team"}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("seed team: %v", err)
	}

	legacyEvent := events.Event{
		ExternalMatchId: "legacy-match",
		Source:          "fotmob",
		Sport:           "football",
		HomeTeamId:      team.TeamId,
		AwayTeamId:      team.TeamId,
		StartTimestamp:  1700000000000,
		StatusType:      "notstarted",
		LeagueId:        legacyPremierLeague,
	}
	shiftedEvent := events.Event{
		ExternalMatchId: "shifted-match",
		Source:          "fotmob",
		Sport:           "football",
		HomeTeamId:      team.TeamId,
		AwayTeamId:      team.TeamId,
		StartTimestamp:  1700000000000,
		StatusType:      "notstarted",
		LeagueId:        shiftedPremierLeague,
	}
	for _, event := range []events.Event{legacyEvent, shiftedEvent} {
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("seed event %s: %v", event.ExternalMatchId, err)
		}
	}

	// The same device is assigned to both rows, which the unique index
	// on (device_id, tournament_id) forbids once the legacy row moved.
	assignments := []tournaments.DeviceTournament{
		{DeviceID: device.ID, TournamentID: legacyPremierLeague},
		{DeviceID: device.ID, TournamentID: shiftedPremierLeague},
		{DeviceID: device.ID, TournamentID: legacyLaLiga},
	}
	for _, assignment := range assignments {
		if err := db.Create(&assignment).Error; err != nil {
			t.Fatalf("seed device assignment: %v", err)
		}
	}

	configs := []tournaments.GlobalTournamentConfig{
		{TournamentID: legacyPremierLeague},
		{TournamentID: legacyLaLiga},
	}
	for _, config := range configs {
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("seed global config: %v", err)
		}
	}

	if err := MigrateFotMobTournamentDuplicates(db); err != nil {
		t.Fatalf("MigrateFotMobTournamentDuplicates: %v", err)
	}

	live := liveTournamentIDs(t, db)
	for _, gone := range []uint{legacyPremierLeague, legacyLaLiga} {
		if containsID(live, gone) {
			t.Errorf("legacy tournament %d is still listed", gone)
		}
	}
	for _, kept := range []uint{
		shiftedPremierLeague,
		shiftedLaLiga,
		legacyEFL,
		legacyAdminCollision,
		shiftedAdminCollision,
		adminTournament,
		scores365League,
	} {
		if !containsID(live, kept) {
			t.Errorf("tournament %d disappeared", kept)
		}
	}

	var legacyRow tournaments.Tournament
	if err := db.Unscoped().First(&legacyRow, legacyPremierLeague).Error; err != nil {
		t.Fatalf("read legacy tournament: %v", err)
	}
	if !legacyRow.DeletedAt.Valid {
		t.Errorf("legacy tournament %d was not soft-deleted", legacyPremierLeague)
	}

	var shiftedRow tournaments.Tournament
	if err := db.First(&shiftedRow, shiftedPremierLeague).Error; err != nil {
		t.Fatalf("read shifted tournament: %v", err)
	}
	if shiftedRow.Name != "Premier League" || shiftedRow.Region != "GB" {
		t.Errorf("shifted tournament changed: %+v", shiftedRow)
	}

	var legacyMatch, shiftedMatch events.Event
	if err := db.First(&legacyMatch, "external_match_id = ?", "legacy-match").Error; err != nil {
		t.Fatalf("read legacy event: %v", err)
	}
	if legacyMatch.LeagueId != shiftedPremierLeague {
		t.Errorf("legacy event league_id = %d, want %d", legacyMatch.LeagueId, shiftedPremierLeague)
	}
	if err := db.First(&shiftedMatch, "external_match_id = ?", "shifted-match").Error; err != nil {
		t.Fatalf("read shifted event: %v", err)
	}
	if shiftedMatch.LeagueId != shiftedPremierLeague {
		t.Errorf("shifted event league_id = %d, want %d", shiftedMatch.LeagueId, shiftedPremierLeague)
	}

	var deviceAssignments []tournaments.DeviceTournament
	if err := db.Where("device_id = ?", device.ID).Find(&deviceAssignments).Error; err != nil {
		t.Fatalf("read device assignments: %v", err)
	}
	if len(deviceAssignments) != 2 {
		t.Fatalf("device has %d assignments, want 2: %+v", len(deviceAssignments), deviceAssignments)
	}
	for _, assignment := range deviceAssignments {
		if assignment.TournamentID == legacyPremierLeague || assignment.TournamentID == legacyLaLiga {
			t.Errorf("assignment still points at the legacy tournament %d", assignment.TournamentID)
		}
	}

	var globalConfigs []tournaments.GlobalTournamentConfig
	if err := db.Find(&globalConfigs).Error; err != nil {
		t.Fatalf("read global configs: %v", err)
	}
	if len(globalConfigs) != 2 {
		t.Fatalf("got %d global configs, want 2: %+v", len(globalConfigs), globalConfigs)
	}
	for _, config := range globalConfigs {
		if config.TournamentID == legacyPremierLeague || config.TournamentID == legacyLaLiga {
			t.Errorf("global config still points at the legacy tournament %d", config.TournamentID)
		}
	}
}

// TestMigrateFotMobTournamentDuplicates_Idempotent makes sure a second
// run does not touch anything: the migration runs on every boot.
func TestMigrateFotMobTournamentDuplicates_Idempotent(t *testing.T) {
	db := openDedupTestDB(t)

	const legacyID = 47
	shiftedID := uint(uint64(legacyID) + tournaments.FotMobTournamentIDBase)

	seedTournament(t, db, legacyID, "Premier League", "premier-league", "GB")
	seedTournament(t, db, shiftedID, "Premier League", "premier-league", "GB")
	seedTournament(t, db, 87, "LaLiga", "laliga", "ES")

	if err := MigrateFotMobTournamentDuplicates(db); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	candidates, err := legacyFotMobTournamentRows(db)
	if err != nil {
		t.Fatalf("list legacy candidates: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.ID == legacyID {
			t.Errorf("the soft-deleted legacy row is still a candidate: %+v", candidate)
		}
	}

	before := dedupSnapshot(t, db)
	if err := MigrateFotMobTournamentDuplicates(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	after := dedupSnapshot(t, db)

	if before != after {
		t.Errorf("second run changed the database:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestMigrateFotMobTournamentDuplicates_NoDuplicates makes sure the
// migration is a no-op on a database without duplicates.
func TestMigrateFotMobTournamentDuplicates_NoDuplicates(t *testing.T) {
	db := openDedupTestDB(t)

	seedTournament(t, db, 1_000_000_001, "Admin Cup", "admin-cup", "")
	seedTournament(t, db, 6_000_000_047, "365scores League", "365scores-league", "AR")
	seedTournament(t, db, 47, "Premier League", "premier-league", "GB")

	before := dedupSnapshot(t, db)
	if err := MigrateFotMobTournamentDuplicates(db); err != nil {
		t.Fatalf("migration: %v", err)
	}
	after := dedupSnapshot(t, db)

	if before != after {
		t.Errorf("migration changed a database without duplicates:\nbefore: %s\nafter:  %s", before, after)
	}
}

// dedupSnapshot renders the state of every table the migration touches
// so two runs can be compared.
func dedupSnapshot(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var builder strings.Builder

	var tournamentsRows []tournaments.Tournament
	if err := db.Unscoped().Order("id ASC").Find(&tournamentsRows).Error; err != nil {
		t.Fatalf("read tournaments: %v", err)
	}
	for _, row := range tournamentsRows {
		fmt.Fprintf(&builder, "tournament %d %q %q deleted=%v\n",
			row.ID, row.Name, row.Region, row.DeletedAt.Valid)
	}

	var assignments []tournaments.DeviceTournament
	if err := db.Unscoped().Order("id ASC").Find(&assignments).Error; err != nil {
		t.Fatalf("read device assignments: %v", err)
	}
	for _, row := range assignments {
		fmt.Fprintf(&builder, "device_tournament %d device=%d tournament=%d deleted=%v\n",
			row.ID, row.DeviceID, row.TournamentID, row.DeletedAt.Valid)
	}

	var configs []tournaments.GlobalTournamentConfig
	if err := db.Unscoped().Order("id ASC").Find(&configs).Error; err != nil {
		t.Fatalf("read global configs: %v", err)
	}
	for _, row := range configs {
		fmt.Fprintf(&builder, "global_config %d tournament=%d deleted=%v\n",
			row.ID, row.TournamentID, row.DeletedAt.Valid)
	}

	var eventRows []events.Event
	if err := db.Unscoped().Order("id ASC").Find(&eventRows).Error; err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, row := range eventRows {
		fmt.Fprintf(&builder, "event %d %q league=%d deleted=%v\n",
			row.ID, row.ExternalMatchId, row.LeagueId, row.DeletedAt.Valid)
	}

	return builder.String()
}
