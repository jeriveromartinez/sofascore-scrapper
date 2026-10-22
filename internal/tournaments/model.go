package tournaments

import "gorm.io/gorm"

// Primary keys of the tournaments table are namespaced.
//
// The table holds two kinds of rows: leagues mirrored from an upstream
// source and tournaments typed by an admin. Every kind owns a
// disjoint band of the primary key:
//
//   - 365scores: scores365.LeagueIDPrefix + competition id (6e9+)
//   - FotMob: the scraper's FotMob base + competition id (8e9+)
//
// Admin rows used to take the table's auto-increment counter, which
// starts at 1 and therefore landed right on top of the upstream
// competition ids. The next scrape tick upserts the upstream league
// on the same primary key, so the tournament the admin created
// silently turned into the upstream league.
const (
	// AdminTournamentIDBase is the first primary key reserved for
	// admin-created tournaments.
	AdminTournamentIDBase uint = 1_000_000_000
	// AdminTournamentIDCeiling is the exclusive upper bound of the
	// reserved admin band.
	AdminTournamentIDCeiling uint = 2_000_000_000
	// FotMobTournamentIDBase is the base of the primary-key band that
	// holds the FotMob leagues. It lives here, next to the other
	// bands, because the data migration that repairs the rows the
	// pre-shift code path wrote has to derive the shifted id from it.
	//
	// The scraper shifts the raw FotMob competition id into this band
	// (see scraper.TournamentID).
	FotMobTournamentIDBase uint64 = 8_000_000_000
)

// IsAdminTournamentID reports whether id belongs to the band reserved
// for admin-created tournaments.
func IsAdminTournamentID(id uint) bool {
	return id >= AdminTournamentIDBase && id < AdminTournamentIDCeiling
}

type Tournament struct {
	gorm.Model
	// idx_tournaments_slug covers the cursor pagination in
	// repository.ListPage (ORDER BY slug ASC, id ASC) and the
	// dashboard load in repository.GetAll (ORDER BY slug ASC).
	// Without it every dashboard load walks the full tournaments
	// table and sorts in memory.
	Name   string `gorm:"column:name;type:longtext" json:"name"`
	Slug   string `gorm:"column:slug;type:longtext;index:idx_tournaments_slug" json:"slug"`
	Region string `gorm:"column:region;type:longtext" json:"region"`
}
