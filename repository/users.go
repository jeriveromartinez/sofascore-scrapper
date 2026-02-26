package repository

import (
	"github.com/jeriveromartinez/sofascore-scrapper/database"
	"github.com/jeriveromartinez/sofascore-scrapper/models"
	"golang.org/x/crypto/bcrypt"
)

func CreateUser(username, email, password string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &models.User{Username: username, Email: email, Password: string(hash)}
	result := db.Create(user)
	return user, result.Error
}

func GetUserByUsername(username string) (*models.User, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var user models.User
	result := db.Where("username = ?", username).First(&user)
	return &user, result.Error
}

func CheckPassword(user *models.User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil
}
