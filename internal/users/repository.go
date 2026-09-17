package users

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// Count returns the total number of user accounts.
func (r *Repository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&User{}).Count(&n).Error
	return n, err
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

// CreateAndMaybeBootstrapAdmin inserts the user and, when the users
// table is empty, also promotes the new row to admin in the same
// transaction. This is the entry point the auth registration handler
// uses for the "first account becomes admin" rule: it must be atomic
// so two concurrent registrations on a fresh install cannot both see
// count==1 and both promote themselves (or, on a slow DB, race past
// the count check and leave the operator without admin).
//
// The promotion is a conditional UPDATE inside the same tx:
//
//	UPDATE users SET role = 'admin' WHERE id = ? AND
//	  NOT EXISTS (SELECT 1 FROM users WHERE role = 'admin')
//
// which makes the bootstrap idempotent and serializable even when
// the wrapping handler is hit concurrently from multiple instances.
// RowsAffected==1 means the new account was promoted; 0 means
// someone else already grabbed the bootstrap slot and the caller
// should leave role at the default.
func (r *Repository) CreateAndMaybeBootstrapAdmin(email, password string) (*User, bool, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, false, err
	}
	user := &User{Email: email, Password: string(hash)}
	promoted := false
	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		res := tx.Exec(
			"UPDATE users SET role = ? WHERE id = ? AND NOT EXISTS (SELECT 1 FROM users WHERE role = ?)",
			RoleAdmin, user.ID, RoleAdmin,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			promoted = true
			user.Role = RoleAdmin
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return user, promoted, nil
}

// UpdatePassword overwrites the stored bcrypt hash for a user. It is
// the storage-layer entry point used by the auth package's lazy-rehash
// on login (when the existing hash is below the current cost).
func (r *Repository) UpdatePassword(id uint, hash string) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("password", hash).Error
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

// SetNotificationsEnabled flips the per-user push feature toggle.
// When enabled flips to true, notifications_enabled_at is stamped
// (audit). When it flips to false, the timestamp is left in place
// so the operator can see when the feature was last used.
//
// The caller is expected to enforce the rule that only the user
// themselves (or an admin) can call this endpoint. This method
// does not check authorization; that is the handler's job.
func (r *Repository) SetNotificationsEnabled(id uint, enabled bool) (*User, error) {
	var user User
	if err := r.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{
		"notifications_enabled": enabled,
	}
	if enabled && user.NotificationsEnabledAt == nil {
		// Stamp in Go rather than via SQL NOW() so the same code
		// path works on MariaDB and SQLite (the test driver).
		now := time.Now()
		updates["notifications_enabled_at"] = now
	}
	if err := r.db.Model(&user).Updates(updates).Error; err != nil {
		return nil, err
	}
	// Re-read so the returned struct reflects the new
	// notifications_enabled_at stamp (the Updates map bypasses
	// GORM's after-update hooks for non-zero fields).
	return r.GetByID(id)
}

// ErrLastAdmin is returned by SetRole when the caller tries to demote
// the only remaining admin. The handler maps this to HTTP 409 so the
// operator knows the request was rejected for safety rather than a
// permissions or validation problem.
var ErrLastAdmin = errors.New("cannot demote the last administrator")

// SetRole updates a single user's role. Only the known roles are
// accepted. Demoting an admin re-counts inside the transaction so
// two concurrent demotions cannot both observe "more than one
// admin" and drop the count to zero, locking the operator out of
// the admin API.
//
// The lock dance matters: the target row is locked with FOR UPDATE,
// then the count of OTHER admins is also locked with FOR UPDATE so
// the snapshot reflects any concurrent committed demotion. Without
// the locks, MariaDB's default REPEATABLE READ isolation would let
// two concurrent transactions both see admins=2, both pass the
// "<= 1" guard, and both demote — leaving zero admins. With the
// locks, the second transaction blocks on the first's row lock and
// re-reads the now-current value of admins when it proceeds.
func (r *Repository) SetRole(ctx context.Context, id uint, role string) (*User, error) {
	if role != RoleUser && role != RoleAdmin {
		return nil, ErrInvalidRole
	}

	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error; err != nil {
			return err
		}

		// No-op: same role. Returning early avoids a no-op UPDATE
		// that would still need to be wrapped in the transaction
		// for the audit trail.
		if user.Role == role {
			return nil
		}

		if user.IsAdmin() && role == RoleUser {
			var otherAdmins int64
			if err := tx.Model(&User{}).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("role = ? AND id <> ?", RoleAdmin, id).
				Count(&otherAdmins).Error; err != nil {
				return err
			}
			if otherAdmins == 0 {
				return ErrLastAdmin
			}
		}

		return tx.Model(&user).Update("role", role).Error
	})
	if err != nil {
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

// bcryptCost is the work factor for new password hashes stored via
// the user repository. Mirrors auth.bcryptCost (12) so that new
// accounts land directly at the current cost and do not require a
// lazy rehash on first login. Keep in sync with
// internal/auth/password.go's bcryptCost.
const bcryptCost = 12

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
