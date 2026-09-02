package users

import (
	"time"

	"gorm.io/gorm"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	gorm.Model
	Email    string `gorm:"size:191;uniqueIndex:idx_users_email;not null"`
	Password string `gorm:"size:255;not null"`
	Role     string `gorm:"size:20;not null;default:'user'"`

	// Push-notifications per-user feature toggle. Default false: the
	// user must opt in before they can create pushes or receive the
	// push section in the web dashboard. Set by the toggle endpoint
	// (PUT /api/admin/v1/users/{id}/notifications).
	NotificationsEnabled   bool       `gorm:"not null;default:false"`
	NotificationsEnabledAt *time.Time `gorm:"null"`
}

// IsAdmin reports whether the user holds administrative privileges.
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}
