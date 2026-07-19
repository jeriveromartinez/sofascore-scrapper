package repository

import (
	"time"

	internalUsers "github.com/jeriveromartinez/sofascore-scrapper/internal/users"
	internalAuth "github.com/jeriveromartinez/sofascore-scrapper/internal/auth"
)

func usersRepo() (*internalUsers.Repository, error) {
	return internalUsers.NewUserRepository()
}

func authRepo() (*internalAuth.AuthRepository, error) {
	return internalAuth.NewAuthRepositoryFromEnv()
}

func GetAllUsers() ([]internalUsers.User, error) {
	repo, err := usersRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetAll()
}

func CreateUser(email, password string) (*internalUsers.User, error) {
	repo, err := usersRepo()
	if err != nil {
		return nil, err
	}
	return repo.Create(email, password)
}

func GetUserByEmail(email string) (*internalUsers.User, error) {
	repo, err := usersRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByEmail(email)
}

func GetUserByID(id uint) (*internalUsers.User, error) {
	repo, err := usersRepo()
	if err != nil {
		return nil, err
	}
	return repo.GetByID(id)
}

func UpdateUser(id uint, email, password string) (*internalUsers.User, error) {
	repo, err := usersRepo()
	if err != nil {
		return nil, err
	}
	return repo.Update(id, email, password)
}

func DeleteUser(id uint) error {
	repo, err := usersRepo()
	if err != nil {
		return err
	}
	return repo.Delete(id)
}

func SaveRefreshToken(userID uint, tokenID string, expiresAt time.Time) error {
	repo, err := authRepo()
	if err != nil {
		return err
	}
	return repo.SaveRefreshToken(userID, tokenID, expiresAt)
}

func IsRefreshTokenActive(userID uint, tokenID string) (bool, error) {
	repo, err := authRepo()
	if err != nil {
		return false, err
	}
	return repo.IsRefreshTokenActive(userID, tokenID)
}

func RevokeRefreshToken(userID uint, tokenID string) error {
	repo, err := authRepo()
	if err != nil {
		return err
	}
	return repo.RevokeRefreshToken(userID, tokenID)
}

func RevokeAllRefreshTokens(userID uint) error {
	repo, err := authRepo()
	if err != nil {
		return err
	}
	return repo.RevokeAllRefreshTokens(userID)
}

func CheckPassword(user *internalUsers.User, password string) bool {
	return internalAuth.CheckPassword(user.Password, password)
}
