// Package seeder provides first-boot data for a fresh database.
package seeder

const (
	// DefaultAdminEmail is the email seeded on first boot.
	DefaultAdminEmail = "admin@local"

	// DefaultAdminPassword is the plaintext password seeded on first
	// boot. It is bcrypt-hashed before being persisted. Operators
	// MUST change it on first login.
	DefaultAdminPassword = "admin1234"

	// defaultAdminBcryptCost matches users.repository.bcryptCost.
	// Kept here (not imported) to avoid a seeder → repo import cycle.
	defaultAdminBcryptCost = 12
)
