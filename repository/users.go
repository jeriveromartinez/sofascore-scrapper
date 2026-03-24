package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"golang.org/x/crypto/bcrypt"
)

func CreateUser(email, password string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &models.User{Email: email, Password: string(hash)}
	result := db.Create(user)
	return user, result.Error
}

func GetUserByEmail(email string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var user models.User
	result := db.Where("email = ?", email).First(&user)
	return &user, result.Error
}

func GetUserByID(id uint) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var user models.User
	result := db.First(&user, id)
	return &user, result.Error
}

func SaveRefreshToken(userID uint, tokenID string, expiresAt time.Time) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	refreshToken := &models.RefreshToken{
		UserID:    userID,
		TokenID:   tokenID,
		ExpiresAt: expiresAt,
	}

	return db.Create(refreshToken).Error
}

func IsRefreshTokenActive(userID uint, tokenID string) (bool, error) {
	db, err := database.GetDB()
	if err != nil {
		return false, err
	}

	var refreshToken models.RefreshToken
	result := db.Where("user_id = ? AND token_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, tokenID, time.Now()).First(&refreshToken)
	if result.Error != nil {
		return false, result.Error
	}

	return true, nil
}

func RevokeRefreshToken(userID uint, tokenID string) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	now := time.Now()
	return db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND token_id = ? AND revoked_at IS NULL", userID, tokenID).
		Update("revoked_at", &now).Error
}

func RevokeAllRefreshTokens(userID uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	now := time.Now()
	return db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", &now).Error
}

func CheckPassword(user *models.User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil
}
