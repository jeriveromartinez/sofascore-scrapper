package playback

import (
	"context"

	"github.com/jeriveromartinez/sofascore-scrapper/internal/devices"
	"gorm.io/gorm"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Start(ctx context.Context, device devices.Device, content string, startedAt int64) (*PlaybackLog, error) {
	return s.repo.Start(ctx, device.ID, content, startedAt)
}

func (s *Service) DB() *gorm.DB {
	return s.repo.db
}
