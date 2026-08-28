//go:build integration

package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&RefreshToken{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedRefreshToken(t *testing.T, db *gorm.DB, userID uint, tokenID string) {
	t.Helper()
	err := db.Create(&RefreshToken{
		UserID:    userID,
		TokenID:   tokenID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}).Error
	if err != nil {
		t.Fatal(err)
	}
}

func isActive(db *gorm.DB, userID uint, tokenID string) bool {
	var token RefreshToken
	result := db.Where("user_id = ? AND token_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, tokenID, time.Now()).First(&token)
	return result.Error == nil
}

func TestRotateRefreshToken_ConcurrentOnlyOneSucceeds(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "old-token-id"
	newID1 := "new-token-1"
	newID2 := "new-token-2"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	seedRefreshToken(t, db, userID, oldID)

	var wg sync.WaitGroup
	results := make(chan error, 2)

	for _, newID := range []string{newID1, newID2} {
		wg.Add(1)
		go func(newID string) {
			defer wg.Done()
			results <- repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
		}(newID)
	}

	wg.Wait()
	close(results)

	successCount := 0
	failCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else if errors.Is(err, ErrInvalidRefreshToken) {
			failCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 success, got %d", successCount)
	}
	if failCount != 1 {
		t.Fatalf("expected exactly 1 failure, got %d", failCount)
	}
}

func TestRotateRefreshToken_InsertFailureRollback(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "old-token-id"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	seedRefreshToken(t, db, userID, oldID)

	newID := "dup-token-id"
	seedRefreshToken(t, db, userID+1, newID)

	err := repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
	if err == nil {
		t.Fatal("expected error from duplicate token_id")
	}

	if !isActive(db, userID, oldID) {
		t.Fatal("old token should remain active after failed rotation")
	}
}

func TestRotateRefreshToken_ZeroRowsAffectedReturnsErrInvalidRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "nonexistent-token"
	newID := "new-token"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	err := repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
}

func TestRotateRefreshToken_AlreadyRevokedReturnsErrInvalidRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "revoked-token"
	newID := "new-token"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	seedRefreshToken(t, db, userID, oldID)

	now := time.Now()
	err := db.Model(&RefreshToken{}).Where("token_id = ?", oldID).Update("revoked_at", now).Error
	if err != nil {
		t.Fatal(err)
	}

	err = repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for revoked token, got %v", err)
	}
}

func TestRotateRefreshToken_ExpiredTokenReturnsErrInvalidRefreshToken(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "expired-token"
	newID := "new-token"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	err := db.Create(&RefreshToken{
		UserID:    userID,
		TokenID:   oldID,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}).Error
	if err != nil {
		t.Fatal(err)
	}

	err = repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for expired token, got %v", err)
	}
}

func TestRotateRefreshToken_SuccessfulRotation(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuthRepository(db)

	userID := uint(1)
	oldID := "old-token"
	newID := "new-token"
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	seedRefreshToken(t, db, userID, oldID)

	err := repo.RotateRefreshToken(context.Background(), userID, oldID, newID, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	if isActive(db, userID, oldID) {
		t.Fatal("old token should be revoked after rotation")
	}

	if !isActive(db, userID, newID) {
		t.Fatal("new token should be active after rotation")
	}
}
