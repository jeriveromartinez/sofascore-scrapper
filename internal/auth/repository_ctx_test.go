package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newRefreshTokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&RefreshToken{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func TestAuthRepositoryRefreshTokenWritesHonourContext(t *testing.T) {
	db := newRefreshTokenTestDB(t)
	repo := NewAuthRepository(db)
	if err := repo.SaveRefreshToken(context.Background(), 1, "live", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.SaveRefreshToken(cancelled, 1, "late", time.Now().Add(time.Hour)); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveRefreshToken error = %v, want context.Canceled", err)
	}
	if err := repo.RevokeRefreshToken(cancelled, 1, "live"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RevokeRefreshToken error = %v, want context.Canceled", err)
	}
	if err := repo.RevokeAllRefreshTokens(cancelled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("RevokeAllRefreshTokens error = %v, want context.Canceled", err)
	}

	var tokens []RefreshToken
	if err := db.Find(&tokens).Error; err != nil {
		t.Fatalf("load tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].TokenID != "live" || tokens[0].RevokedAt != nil {
		t.Fatalf("tokens = %+v, want only the untouched live token", tokens)
	}
}
