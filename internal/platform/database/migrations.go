package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jeriveromartinez/sofascore-scrapper/migrations"
)

const lockID = "590872375"

func Migrate(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}

	_, err := db.ExecContext(ctx, fmt.Sprintf("SELECT GET_LOCK('%s', 0)", lockID))
	if err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), fmt.Sprintf("SELECT RELEASE_LOCK('%s')", lockID)); err != nil {
			log.Printf("migrations: release lock: %v", err)
		}
	}()

	src, err := iofs.New(migrations.FS(), ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return fmt.Errorf("migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}
