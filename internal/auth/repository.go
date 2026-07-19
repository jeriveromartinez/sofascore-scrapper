package auth

import (
	"time"

	"gorm.io/gorm"
)

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

func (r *AuthRepository) IsRefreshTokenActive(userID uint, tokenID string) (bool, error) {
	var token RefreshToken
	result := r.db.Where("user_id = ? AND token_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, tokenID, time.Now()).First(&token)
	if result.Error != nil {
		return false, result.Error
	}
	return true, nil
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


