//go:build integration

package realtime

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openDriver is a thin indirection so the integration tests can use
// the same sqlite driver the rest of the project does
// (github.com/glebarez/sqlite, no CGO). Keeping it in a build-tagged
// file means the unit-test build never pulls in the sqlite driver
// and pays zero import cost.
func openDriver(dsn string) gorm.Dialector {
	return sqlite.Open(dsn)
}
