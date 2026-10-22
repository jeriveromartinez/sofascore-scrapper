package users

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/go-sql-driver/mysql"
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

func (r *Repository) GetAll(ctx context.Context) ([]User, error) {
	var users []User
	result := r.db.WithContext(ctx).Order("email ASC").Find(&users)
	if result.Error != nil {
		return nil, result.Error
	}
	return users, nil
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
func (r *Repository) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&User{}).Count(&n).Error
	return n, err
}

func (r *Repository) Create(ctx context.Context, email, password string) (*User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &User{Email: NormalizeEmail(email), Password: string(hash)}
	result := r.db.WithContext(ctx).Create(user)
	if result.Error != nil {
		return nil, result.Error
	}
	return user, nil
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
//
// On InnoDB the NOT EXISTS scan takes shared locks on the rows other
// in-flight registrations just inserted, so concurrent registrations
// on a fresh install deadlock and all but one are rolled back. Those
// transactions changed nothing and are replayed.
func (r *Repository) CreateAndMaybeBootstrapAdmin(ctx context.Context, email, password string) (*User, bool, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, false, err
	}
	for attempt := 1; ; attempt++ {
		user, promoted, err := r.createAndMaybePromote(ctx, NormalizeEmail(email), hash)
		if err == nil {
			return user, promoted, nil
		}
		if !isDeadlock(err) || attempt == maxBootstrapAttempts {
			return nil, false, err
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(time.Duration(rand.IntN(10*attempt)+1) * time.Millisecond):
		}
	}
}

// maxBootstrapAttempts bounds the deadlock replays in
// CreateAndMaybeBootstrapAdmin.
const maxBootstrapAttempts = 10

func (r *Repository) createAndMaybePromote(ctx context.Context, email, hash string) (*User, bool, error) {
	user := &User{Email: email, Password: hash}
	promoted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

const mysqlErrDeadlock = 1213

func isDeadlock(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDeadlock
}

// UpdatePassword overwrites the stored bcrypt hash for a user. It is
// the storage-layer entry point used by the auth package's lazy-rehash
// on login (when the existing hash is below the current cost).
func (r *Repository) UpdatePassword(ctx context.Context, id uint, hash string) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("password", hash).Error
}

// GetByEmail returns nil, nil when the email is not in the table,
// rather than returning a partially-zeroed struct + gorm.ErrRecordNotFound
// — callers should not have to remember the not-found convention.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", NormalizeEmail(email)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetByID returns nil, nil on gorm.ErrRecordNotFound so callers can
// distinguish "no such user" from "DB error" without remembering
// to errors.Is the error.
func (r *Repository) GetByID(ctx context.Context, id uint) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) Update(ctx context.Context, id uint, email, password string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}

	user.Email = NormalizeEmail(email)
	if password != "" {
		hash, err := hashPassword(password)
		if err != nil {
			return nil, err
		}
		user.Password = hash
	}

	if err := r.db.WithContext(ctx).Save(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// ErrInvalidRole is returned when a role outside the known set is requested.
var ErrInvalidRole = errors.New("invalid role")

// CountAdmins returns how many users currently hold the admin role.
func (r *Repository) CountAdmins(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("role = ?", RoleAdmin).Count(&count).Error
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
func (r *Repository) SetNotificationsEnabled(ctx context.Context, id uint, enabled bool) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Model(&user).Updates(updates).Error; err != nil {
		return nil, err
	}
	// Re-read so the returned struct reflects the new
	// notifications_enabled_at stamp (the Updates map bypasses
	// GORM's after-update hooks for non-zero fields).
	return r.GetByID(ctx, id)
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

func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
