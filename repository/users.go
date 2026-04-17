package repository

import (
	"time"

	"github.com/jeriveromartinez/sofascore-scrapper/libs/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func GetAllUsers() ([]models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	var users []models.User
	result := db.Order("email ASC").Find(&users)
	return users, result.Error
}

func CreateUser(email, password string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
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

func UpdateUser(id uint, email, password string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		return nil, err
	}

	user.Email = email
	if password != "" {
		hash, err := hashPassword(password)
		if err != nil {
			return nil, err
		}
		user.Password = hash
	}

	result := db.Save(&user)
	return &user, result.Error
}

func DeleteUser(id uint) error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&models.User{}, id).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.Device{}).
			Where("user_id = ?", id).
			Updates(map[string]any{"user_id": nil}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", id).Delete(&models.Domain{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", id).Delete(&models.RefreshToken{}).Error; err != nil {
			return err
		}

		return tx.Delete(&models.User{}, id).Error
	})
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

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}
