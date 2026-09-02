package auth

import (
	"golang.org/x/crypto/bcrypt"
	"github.com/jeriveromartinez/sofascore-scrapper/internal/users"
)

const (
	// BcryptCost is the work factor for new password hashes. Cost 12
	// is the project target for 2026 hardware (~400 ms per hash on a
	// modern CPU). Existing cost-10 hashes are re-hashed lazily on
	// the next successful login via VerifyAndUpgrade.
	//
	// Exported so the seeder package can use the same value without
	// duplicating it. The users package cannot import this (cycle
	// auth -> users in handler.go); users.repository.bcryptCost is a
	// local mirror that must be bumped together with this constant.
	BcryptCost = 12
)

// ValidatePassword is the storage-agnostic policy check used by
// the auth and users packages. The implementation lives in the
// users package so the admin create / update handlers can call it
// without an import cycle.
func ValidatePassword(password string) error {
	return users.ValidatePassword(password)
}

// CheckPassword reports whether the plaintext matches the bcrypt
// hash. Constant-time relative to hash parsing, not to password
// length — the cost of the hash dominates.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// HashPassword returns a bcrypt hash of the plaintext at the
// project's standard cost. Callers should NOT re-hash the output —
// it is safe to store directly.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyAndUpgrade is the "lazy rehash" helper used during login.
// It returns the bcrypt error (or nil) for the comparison, and a
// non-empty new hash ONLY when the supplied hash matched the
// plaintext AND was below the current cost. Callers persist the
// non-empty new hash in place of the stored one.
//
// Behavior summary:
//   - wrong password          -> newHash = "", err = bcrypt.ErrMismatchedHashAndPassword
//   - correct + current cost  -> newHash = "", err = nil
//   - correct + legacy cost    -> newHash = re-hashed at BcryptCost, err = nil
func VerifyAndUpgrade(hash, password string) (newHash string, err error) {
	if err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", err
	}
	cost, costErr := bcrypt.Cost([]byte(hash))
	if costErr != nil {
		// Hash is valid (CompareHashAndPassword passed) but the cost
		// could not be parsed — treat as a current-cost match and do
		// not upgrade. The cost format is stable across bcrypt
		// implementations, so this branch should never fire in
		// practice; if it does, skipping the upgrade is the safer
		// default than rejecting a valid login.
		return "", nil
	}
	if cost >= BcryptCost {
		return "", nil
	}
	upgraded, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	return upgraded, nil
}
