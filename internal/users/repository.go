package users

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll() ([]User, error) {
	var users []User
	result := r.db.Order("email ASC").Find(&users)
	return users, result.Error
}

func (r *Repository) ListPage(ctx context.Context, email string, id uint, limit int) ([]User, bool, error) {
	query := r.db.WithContext(ctx).Order("email ASC, id ASC")
	if email != "" {
		query = query.Where("email > ? OR (email = ? AND id > ?)", email, email, id)
	}
	var rows []User
	err := query.Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

func (r *Repository) Create(email, password string) (*User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &User{Email: email, Password: string(hash)}
	result := r.db.Create(user)
	return user, result.Error
}

func (r *Repository) GetByEmail(email string) (*User, error) {
	var user User
	result := r.db.Where("email = ?", email).First(&user)
	return &user, result.Error
}

func (r *Repository) GetByID(id uint) (*User, error) {
	var user User
	result := r.db.First(&user, id)
	return &user, result.Error
}

func (r *Repository) Update(id uint, email, password string) (*User, error) {
	var user User
	if err := r.db.First(&user, id).Error; err != nil {
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

	result := r.db.Save(&user)
	return &user, result.Error
}

// ErrInvalidRole is returned when a role outside the known set is requested.
var ErrInvalidRole = errors.New("invalid role")

// CountAdmins returns how many users currently hold the admin role.
func (r *Repository) CountAdmins() (int64, error) {
	var count int64
	err := r.db.Model(&User{}).Where("role = ?", RoleAdmin).Count(&count).Error
	return count, err
}

// SetRole updates a single user's role. Only the known roles are accepted.
func (r *Repository) SetRole(id uint, role string) (*User, error) {
	if role != RoleUser && role != RoleAdmin {
		return nil, ErrInvalidRole
	}

	var user User
	if err := r.db.First(&user, id).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&user).Update("role", role).Error; err != nil {
		return nil, err
	}
	user.Role = role
	return &user, nil
}

func (r *Repository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&User{}, id).Error; err != nil {
			return err
		}

		if err := tx.Exec("UPDATE devices SET user_id = NULL WHERE user_id = ?", id).Error; err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM domains WHERE user_id = ?", id).Error; err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", id).Error; err != nil {
			return err
		}

		return tx.Delete(&User{}, id).Error
	})
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
