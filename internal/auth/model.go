package auth

import (
	"time"

	"gorm.io/gorm"
)

type RefreshToken struct {
	gorm.Model
	UserID    uint       `gorm:"column:user_id;not null;index:idx_refresh_tokens_user_id"`
	TokenID   string     `gorm:"column:token_id;size:64;uniqueIndex:idx_refresh_tokens_token_id;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;index:idx_refresh_tokens_expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at;index:idx_refresh_tokens_revoked_at"`
}
