// Package seeder provides first-boot data for a fresh database.
package seeder

import "github.com/jeriveromartinez/sofascore-scrapper/internal/auth"

const (
	// DefaultAdminEmail is the email seeded on first boot.
	DefaultAdminEmail = "admin@local"

	// DefaultAdminPassword is the plaintext password seeded on first
	// boot. It is bcrypt-hashed before being persisted. Operators
	// MUST change it on first login.
	DefaultAdminPassword = "admin1234"

	// defaultAdminBcryptCost matches auth.BcryptCost so the seeded
	// admin password lands at the current cost without a lazy rehash
	// on first login.
	defaultAdminBcryptCost = auth.BcryptCost
)
