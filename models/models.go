package models

import "github.com/jeriveromartinez/sofascore-scrapper/libs/database"

func Migrate() {
	db, err := database.GetDB()
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&SofaScoreEvent{}, &Tournament{}, &Team{}, &User{}, &Device{}, &PlaybackLog{}, &ApkVersion{}, &DeviceTournament{}, &GlobalTournamentConfig{}); err != nil {
		panic(err)
	}
}
