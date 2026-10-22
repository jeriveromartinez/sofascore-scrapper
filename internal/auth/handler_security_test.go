package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	pb "github.com/jeriveromartinez/scores365-scrapper/internal/gen/api"
	"github.com/jeriveromartinez/scores365-scrapper/internal/users"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const testPassword = "correct-horse-battery"

type authTestEnv struct {
	db          *gorm.DB
	router      *gin.Engine
	invitations *InvitationStore
	userRepo    *users.Repository
	miniredis   *miniredis.Miniredis
}

func newAuthTestEnv(t *testing.T) *authTestEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:auth_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &RefreshToken{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	tokens, err := NewTokenService("test-secret-that-is-long-enough-for-jwt")
	if err != nil {
		t.Fatalf("token service: %v", err)
	}

	invitations := NewInvitationStore(client)
	handler := NewAuthHandler(NewAuthRepository(db), users.NewRepository(db), tokens, invitations)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/users/register", handler.handleRegister)
	router.POST("/users/login", handler.handleLogin)

	return &authTestEnv{
		db:          db,
		router:      router,
		invitations: invitations,
		userRepo:    users.NewRepository(db),
		miniredis:   mr,
	}
}

func (e *authTestEnv) post(t *testing.T, path string, req *pb.AuthRequest) *httptest.ResponseRecorder {
	t.Helper()
	return e.postWithContext(t, context.Background(), path, req)
}

func (e *authTestEnv) postWithContext(t *testing.T, ctx context.Context, path string, req *pb.AuthRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, httpReq)
	return rec
}

// TestHandleRegister_FailedUserCreationKeepsInvitationUsable covers
// the invitation lifecycle: the token is reserved while the user is
// created and released when that fails, so a transient error does not
// burn the invitation.
func TestHandleRegister_FailedUserCreationKeepsInvitationUsable(t *testing.T) {
	env := newAuthTestEnv(t)

	token, _, err := env.invitations.Create(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	// A user with this email already exists, so the registration
	// fails on the unique index.
	existing := &users.User{Email: "taken@example.com", Password: "x", Role: users.RoleUser}
	if err := env.db.Create(existing).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	rec := env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "taken@example.com",
		Password:        testPassword,
		InvitationToken: token,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%q)", rec.Code, rec.Body.String())
	}

	// The same invitation must still work for a different address.
	rec = env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "fresh@example.com",
		Password:        testPassword,
		InvitationToken: token,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestHandleRegister_CanceledRequestStillReleasesInvitation covers the
// client-disconnect case: the request context is canceled while the
// registration is in flight. The reservation must be released anyway,
// otherwise the invitation is stranded in the reserved key and can
// never be used again.
func TestHandleRegister_CanceledRequestStillReleasesInvitation(t *testing.T) {
	env := newAuthTestEnv(t)

	token, _, err := env.invitations.Create(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel the request as soon as the invitation is reserved: the
	// user creation then fails with the canceled context, exactly like
	// a client that hung up mid-request.
	reserved := reservedInvitationKey(token)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if env.miniredis.Exists(reserved) {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	rec := env.postWithContext(t, ctx, "/users/register", &pb.AuthRequest{
		Email:           "canceled@example.com",
		Password:        testPassword,
		InvitationToken: token,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%q)", rec.Code, rec.Body.String())
	}

	// The invitation must still be usable by a later registration.
	rec = env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "fresh@example.com",
		Password:        testPassword,
		InvitationToken: token,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestHandleRegister_NormalizesEmail covers the registration/login
// symmetry: an address registered with surrounding whitespace must
// still resolve on login.
func TestHandleRegister_NormalizesEmail(t *testing.T) {
	env := newAuthTestEnv(t)

	token, _, err := env.invitations.Create(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	rec := env.post(t, "/users/register", &pb.AuthRequest{
		Email:           "  spaced@example.com  ",
		Password:        testPassword,
		InvitationToken: token,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}

	var stored users.User
	if err := env.db.Where("email = ?", "spaced@example.com").First(&stored).Error; err != nil {
		t.Fatalf("stored email is not normalized: %v", err)
	}

	rec = env.post(t, "/users/login", &pb.AuthRequest{
		Email:    "spaced@example.com",
		Password: testPassword,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
}

// TestHandleLogin_MissingUserTakesSameTimeAsWrongPassword covers the
// user-enumeration side channel: the login path must run a bcrypt
// comparison even when the account does not exist.
func TestHandleLogin_MissingUserTakesSameTimeAsWrongPassword(t *testing.T) {
	env := newAuthTestEnv(t)

	if _, err := env.userRepo.Create(context.Background(), "known@example.com", testPassword); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Warm the throwaway hash up so the measurement compares two
	// bcrypt comparisons and not a hash generation plus a comparison.
	VerifyDummy("warmup")

	start := time.Now()
	rec := env.post(t, "/users/login", &pb.AuthRequest{
		Email:    "known@example.com",
		Password: "wrong-password-value",
	})
	wrongPasswordDuration := time.Since(start)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	start = time.Now()
	rec = env.post(t, "/users/login", &pb.AuthRequest{
		Email:    "unknown@example.com",
		Password: "wrong-password-value",
	})
	missingUserDuration := time.Since(start)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	if missingUserDuration < wrongPasswordDuration/2 {
		t.Fatalf("missing-user login took %s, wrong-password login took %s: the response time leaks account existence",
			missingUserDuration, wrongPasswordDuration)
	}
}

// TestInvitationStore_ReleaseRestoresUsableToken covers the
// reservation lifecycle at the store level.
func TestInvitationStore_ReleaseRestoresUsableToken(t *testing.T) {
	env := newAuthTestEnv(t)
	ctx := context.Background()

	token, _, err := env.invitations.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := env.invitations.Reserve(ctx, token); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := env.invitations.Release(ctx, token); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := env.invitations.Consume(ctx, token); err != nil {
		t.Fatalf("consume after release: %v", err)
	}
}

// TestInvitationStore_CommitConsumesReservation covers the commit
// path: a committed invitation cannot be used again.
func TestInvitationStore_CommitConsumesReservation(t *testing.T) {
	env := newAuthTestEnv(t)
	ctx := context.Background()

	token, _, err := env.invitations.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := env.invitations.Reserve(ctx, token); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := env.invitations.Commit(ctx, token); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := env.invitations.Consume(ctx, token); err == nil {
		t.Fatal("committed invitation was still usable")
	}
}

// TestInvitationStore_ReserveBlocksSecondClaim covers the
// concurrency guard: a reserved invitation cannot be reserved twice.
func TestInvitationStore_ReserveBlocksSecondClaim(t *testing.T) {
	env := newAuthTestEnv(t)
	ctx := context.Background()

	token, _, err := env.invitations.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := env.invitations.Reserve(ctx, token); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := env.invitations.Reserve(ctx, token); err == nil {
		t.Fatal("second reservation succeeded, want ErrInvalidInvitation")
	}
}

// TestRefreshTokenCleanup_PurgesExpiredTokens covers the cleanup of
// the refresh_tokens table.
func TestRefreshTokenCleanup_PurgesExpiredTokens(t *testing.T) {
	env := newAuthTestEnv(t)
	ctx := context.Background()
	repo := NewAuthRepository(env.db)

	expired := &RefreshToken{UserID: 1, TokenID: "expired", ExpiresAt: time.Now().Add(-time.Hour)}
	live := &RefreshToken{UserID: 1, TokenID: "live", ExpiresAt: time.Now().Add(time.Hour)}
	if err := env.db.Create(expired).Error; err != nil {
		t.Fatalf("seed expired: %v", err)
	}
	if err := env.db.Create(live).Error; err != nil {
		t.Fatalf("seed live: %v", err)
	}

	cleanup := NewRefreshTokenCleanup(repo)
	deleted, err := cleanup.Run(ctx)
	if err != nil {
		t.Fatalf("run cleanup: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	var remaining int64
	if err := env.db.Model(&RefreshToken{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining rows = %d, want 1", remaining)
	}
}

// TestRefreshTokenCleanup_RunIfDueThrottles covers the throttle: two
// consecutive calls must not purge twice in a row.
func TestRefreshTokenCleanup_RunIfDueThrottles(t *testing.T) {
	env := newAuthTestEnv(t)
	ctx := context.Background()
	repo := NewAuthRepository(env.db)

	expired := &RefreshToken{UserID: 1, TokenID: "expired", ExpiresAt: time.Now().Add(-time.Hour)}
	if err := env.db.Create(expired).Error; err != nil {
		t.Fatalf("seed expired: %v", err)
	}

	cleanup := NewRefreshTokenCleanup(repo)
	deleted, err := cleanup.RunIfDue(ctx)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	if err := env.db.Create(expired).Error; err != nil {
		t.Fatalf("reseed expired: %v", err)
	}
	deleted, err = cleanup.RunIfDue(ctx)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("second run deleted = %d, want 0 (throttled)", deleted)
	}
}
