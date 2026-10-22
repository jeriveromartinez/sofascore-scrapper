package auth

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
)

// TestHandleRegister_AdminPromotionIsLogged covers the bootstrap of
// the first account: the promotion happened inside the registration
// transaction but nothing reported it, so an operator could not tell
// which account became the admin of a fresh install.
func TestHandleRegister_AdminPromotionIsLogged(t *testing.T) {
	env := newAuthTestEnv(t)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	firstToken, _, err := env.invitations.Create(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	rec := env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "first@example.com",
		Password:        testPassword,
		InvitationToken: firstToken,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "promoted to admin") {
		t.Fatalf("the admin promotion was not logged: %s", logs.String())
	}

	// The second account must not be promoted: only the bootstrap
	// slot is reported.
	logs.Reset()
	secondToken, _, err := env.invitations.Create(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	rec = env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "second@example.com",
		Password:        testPassword,
		InvitationToken: secondToken,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), "promoted to admin") {
		t.Fatalf("a second account was reported as promoted: %s", logs.String())
	}
}
