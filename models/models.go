package models

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/apk"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func Migrate() {
	db, err := database.GetDB()
	if err != nil {
		panic(err)
	}

	if err := db.AutoMigrate(
		&SofaScoreEvent{},
		&Tournament{},
		&Team{},
		&User{},
		&Domain{},
		&RefreshToken{},
		&Device{},
		&playback.PlaybackLog{},
		&apk.ApkVersion{},
		&DeviceTournament{},
		&GlobalTournamentConfig{},
		&ContentStat{},
		&CrashReport{},
	); err != nil {
		panic(err)
	}
}
