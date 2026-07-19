package app

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/domains"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/events"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/reporting"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/tournaments"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&events.Event{},
		&tournaments.Tournament{},
		&events.Team{},
		&users.User{},
		&domains.Domain{},
		&auth.RefreshToken{},
		&devices.Device{},
		&playback.PlaybackLog{},
		&apk.ApkVersion{},
		&tournaments.DeviceTournament{},
		&tournaments.GlobalTournamentConfig{},
		&reporting.ContentStat{},
		&reporting.CrashReport{},
	)
}
