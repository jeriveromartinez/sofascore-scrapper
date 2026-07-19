package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/internal/playback"
	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
)

func playbackRepo() (*playback.Repository, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	return playback.NewRepository(db), nil
}

func LogPlayback(deviceID uint, content string, startedAt int64) (*playback.PlaybackLog, error) {
	repo, err := playbackRepo()
	if err != nil {
		return nil, err
	}
	return repo.Log(deviceID, content, startedAt)
}

func UpdatePlaybackEnd(id uint, endedAt int64) error {
	repo, err := playbackRepo()
	if err != nil {
		return err
	}
	return repo.UpdateEnd(id, endedAt)
}

func GetList(page, limit int) ([]*playback.PlaybackLog, error) {
	repo, err := playbackRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetList(page, limit)
}

func TotalCount() int64 {
	repo, err := playbackRepo()
	if err != nil {
		return 0
	}
	return repo.TotalCount()
}
