package users

import "gorm.io/gorm"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	gorm.Model
	Email    string `gorm:"uniqueIndex;not null"`
	Password string `gorm:"not null"`
	Role     string `gorm:"size:20;not null;default:user"`
}

// IsAdmin reports whether the user holds administrative privileges.
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}
