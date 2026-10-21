package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrInvalidRefreshToken = errors.New("invalid refresh token")

type AuthRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) SaveRefreshToken(userID uint, tokenID string, expiresAt time.Time) error {
	token := &RefreshToken{
		UserID:    userID,
		TokenID:   tokenID,
		ExpiresAt: expiresAt,
	}
	return r.db.Create(token).Error
}

func (r *AuthRepository) RevokeRefreshToken(userID uint, tokenID string) error {
	now := time.Now()
	return r.db.Model(&RefreshToken{}).
		Where("user_id = ? AND token_id = ? AND revoked_at IS NULL", userID, tokenID).
		Update("revoked_at", &now).Error
}

func (r *AuthRepository) RevokeAllRefreshTokens(userID uint) error {
	now := time.Now()
	return r.db.Model(&RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

func (r *AuthRepository) RotateRefreshToken(ctx context.Context, userID uint, oldID, newID string, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		result := tx.Model(&RefreshToken{}).
			Where("user_id = ? AND token_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, oldID, now).
			Update("revoked_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvalidRefreshToken
		}
		return tx.Create(&RefreshToken{UserID: userID, TokenID: newID, ExpiresAt: expiresAt}).Error
	})
}
