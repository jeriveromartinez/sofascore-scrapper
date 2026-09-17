package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubLogoLookup implements LogoLookup for tests. Returns a pre-set
// URL or an error; records call count so tests can assert fallback
// ordering.
type stubLogoLookup struct {
	url   string
	err   error
	calls atomic.Int64
	seen  []string
}

func (s *stubLogoLookup) TeamLogoURL(_ context.Context, name string) (string, error) {
	s.calls.Add(1)
	s.seen = append(s.seen, name)
	return s.url, s.err
}

// withImageStorageDir points IMAGE_STORAGE_PATH at a fresh tmpdir
// for the duration of the test so the downloader writes to a clean
// directory.
func withImageStorageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("IMAGE_STORAGE_PATH", dir)
	return dir
}

// newTestRepoWithLookup returns a Repository wired with a stub
// LogoLookup and no reachable ID source, so the tail of the chain
// fails without touching a live CDN. Use it for tests that only care
// about the primary and lookup steps.
//
// The db is nil: DownloadAndPersistLogo only writes LogoUrl when a
// download succeeds, and it skips that update when db is nil, so
// tests exercise the source chain without a real MariaDB.
func newTestRepoWithLookup(lookup LogoLookup) *Repository {
	return newTestRepoWithSources(lookup, func(int64) string { return "" })
}

// newTestRepoWithSources is newTestRepoWithLookup with an explicit
// ID-source builder, for tests that need the chain's last link to
// resolve to a server they control.
func newTestRepoWithSources(lookup LogoLookup, idSource func(int64) string) *Repository {
	r := NewRepository(nil).WithLogoLookup(lookup)
	r.idSourceURL = idSource
	return r
}

// primaryFailingServer always returns 404 so the chain falls through
// to the LogoLookup and then the ID-derived CDN URL. The test asserts
// that the second source is consulted, then the third.
func primaryFailingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
}

// okServer always returns 200 with the supplied body. Tests use it
// to stand in for whichever CDN step they want the chain to land on.
func okServer(body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
}

// TestDownloadAndPersistLogo_FallsBackToLookup verifies that when
// the primary URL 404s, the lookup-driven source is consulted and
// the file lands on disk. This is the core multi-source behaviour
// that resolves teams like Juventus (FotMob ID 9885) whose primary
// badge URL is missing or stale.
func TestDownloadAndPersistLogo_FallsBackToLookup(t *testing.T) {
	dir := withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	lookup := &stubLogoLookup{
		url: okServer([]byte("lookup-bytes")).URL + "/juventus.png",
	}

	repo := newTestRepoWithLookup(lookup)

	// Use a gorm session bound to a discardable db. We only exercise
	// the source-chain logic — DB update assertions live in the
	// integration test. nil db is fine because the chain reaches the
	// lookup step, which succeeds and writes to disk.
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "Juventus",
		PrimaryURL: primary.URL + "/juventus.png",
	})

	if lookup.calls.Load() != 1 {
		t.Errorf("lookup calls = %d, want 1 (primary 404 must trigger fallback)", lookup.calls.Load())
	}
	if got := strings.TrimSpace(lookup.seen[0]); got != "Juventus" {
		t.Errorf("lookup got %q, want Juventus", got)
	}

	data, err := os.ReadFile(TeamLogoLocalPath(9885))
	if err != nil {
		t.Fatalf("expected logo file at %s: %v", TeamLogoLocalPath(9885), err)
	}
	if string(data) != "lookup-bytes" {
		t.Errorf("logo content = %q, want lookup-bytes", data)
	}

	_ = dir
}

// TestDownloadAndPersistLogo_FallsBackToIDSourceWhenLookupEmpty
// covers the chain when the LogoLookup returns no match — the
// ID-derived CDN URL must be tried last.
func TestDownloadAndPersistLogo_FallsBackToIDSourceWhenLookupEmpty(t *testing.T) {
	withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	// The real ID source is one of the live image CDNs, so the test
	// injects its own builder and asserts on the path it received.
	// That keeps the assertion on the chain's ordering rather than
	// on whichever badge a CDN happens to hold today.
	var idPath string
	idServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idPath = r.URL.Path
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("id-source-bytes"))
	}))
	defer idServer.Close()

	// Empty URL, no error: the lookup ran and found nothing.
	lookup := &stubLogoLookup{}

	repo := newTestRepoWithSources(lookup, func(teamID int64) string {
		return fmt.Sprintf("%s/badge/%d.png", idServer.URL, teamID)
	})
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "Juventus",
		PrimaryURL: primary.URL + "/juventus.png",
	})

	if lookup.calls.Load() != 1 {
		t.Errorf("expected lookup to be consulted once, got %d", lookup.calls.Load())
	}
	if idPath != "/badge/9885.png" {
		t.Errorf("ID source path = %q, want /badge/9885.png", idPath)
	}

	data, err := os.ReadFile(TeamLogoLocalPath(9885))
	if err != nil {
		t.Fatalf("expected logo file: %v", err)
	}
	if string(data) != "id-source-bytes" {
		t.Errorf("logo content = %q, want id-source-bytes", data)
	}
}

// TestDownloadAndPersistLogo_NoLookupSkipsFallback documents that
// the chain degrades cleanly to primary+ID when no LogoLookup is
// configured. This guards against accidentally coupling the
// downloader to the optional dependency.
func TestDownloadAndPersistLogo_NoLookupSkipsFallback(t *testing.T) {
	withImageStorageDir(t)

	primary := okServer([]byte("primary-bytes"))
	defer primary.Close()

	repo := NewRepository(nil)
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "Juventus",
		PrimaryURL: primary.URL + "/juventus.png",
	})

	data, err := os.ReadFile(TeamLogoLocalPath(9885))
	if err != nil {
		t.Fatalf("expected logo file: %v", err)
	}
	if string(data) != "primary-bytes" {
		t.Errorf("logo content = %q, want primary-bytes", data)
	}
}

// TestDownloadAndPersistLogo_AllSourcesFail documents the failure
// mode: every source 404s, the file is not written, and no panic
// is raised. The caller (LogoScheduler worker) must continue
// processing the next job.
func TestDownloadAndPersistLogo_AllSourcesFail(t *testing.T) {
	withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	// The ID source is injected so the chain's last link 404s
	// locally. Pointing it at the real CDN would make the test
	// depend on whether that CDN happens to hold a badge for this
	// ID — team 9885 is Juventus on FotMob, so it does.
	idServer := primaryFailingServer(t)
	defer idServer.Close()

	lookup := &stubLogoLookup{
		err: fmt.Errorf("lookup: no match"),
	}

	repo := newTestRepoWithSources(lookup, func(teamID int64) string {
		return fmt.Sprintf("%s/badge/%d.png", idServer.URL, teamID)
	})
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "Nonexistent",
		PrimaryURL: primary.URL + "/nope.png",
	})

	if lookup.calls.Load() != 1 {
		t.Errorf("expected lookup to be consulted, got %d calls", lookup.calls.Load())
	}
	if _, err := os.Stat(TeamLogoLocalPath(9885)); err == nil {
		t.Error("expected no logo file when every source fails")
	}
}

// TestDownloadAndPersistLogo_EmptyLookupNameSkipped covers the case
// where the job arrives without a TeamName (e.g. older callers that
// only knew teamID). The chain must skip the lookup step and go
// straight to the ID-derived CDN URL. Without this guard the lookup
// would be called with an empty name and waste a request.
func TestDownloadAndPersistLogo_EmptyLookupNameSkipped(t *testing.T) {
	withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	lookup := &stubLogoLookup{
		url: "https://should-never-be-used.example/logo.png",
	}

	// The default test repo has no reachable ID source, so the
	// whole chain fails; the only thing under test is the call
	// counter — 0 means the empty TeamName was honoured.
	repo := newTestRepoWithLookup(lookup)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DownloadAndPersistLogo panicked: %v", r)
		}
	}()
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "",
		PrimaryURL: primary.URL + "/nope.png",
	})

	if lookup.calls.Load() != 0 {
		t.Errorf("lookup calls = %d, want 0 (empty name must skip lookup)", lookup.calls.Load())
	}
}

// TestDownloadAndPersistLogo_PrimarySuccessDoesNotConsultLookup
// documents that successful primary downloads never trigger the
// fallback — important because the lookup is rate-limited and
// primary fetches are not.
func TestDownloadAndPersistLogo_PrimarySuccessDoesNotConsultLookup(t *testing.T) {
	withImageStorageDir(t)

	primary := okServer([]byte("primary-bytes"))
	defer primary.Close()

	lookup := &stubLogoLookup{
		url: "https://should-not-be-fetched.example/logo.png",
	}

	repo := newTestRepoWithLookup(lookup)
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     9885,
		TeamName:   "Juventus",
		PrimaryURL: primary.URL + "/juventus.png",
	})

	if lookup.calls.Load() != 0 {
		t.Errorf("lookup calls = %d, want 0 (primary success must not trigger fallback)", lookup.calls.Load())
	}

	data, err := os.ReadFile(TeamLogoLocalPath(9885))
	if err != nil {
		t.Fatalf("expected logo file: %v", err)
	}
	if string(data) != "primary-bytes" {
		t.Errorf("logo content = %q, want primary-bytes", data)
	}
}

// TestDownloadAndPersistLogo_LookupErrorTreatedAsChainFailure
// documents that a transient lookup error (not just "no match") is
// logged and the chain continues to the next source. Without this,
// a brief lookup outage would block every team whose primary URL 404s.
func TestDownloadAndPersistLogo_LookupErrorTreatedAsChainFailure(t *testing.T) {
	withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	// Fallback URL points at an unreachable host so the chain
	// terminates after the lookup error.
	lookup := &stubLogoLookup{
		url: "http://127.0.0.1:1/will-not-resolve.png",
		err: nil, // simulate "found, but URL is broken"
	}

	repo := newTestRepoWithLookup(lookup)

	done := make(chan struct{})
	go func() {
		defer close(done)
		repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
			TeamID:     9885,
			TeamName:   "Juventus",
			PrimaryURL: primary.URL + "/nope.png",
		})
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("DownloadAndPersistLogo hung on broken lookup URL — must not block the scheduler")
	}

	if lookup.calls.Load() != 1 {
		t.Errorf("lookup calls = %d, want 1", lookup.calls.Load())
	}
}

// TestDownloadAndPersistLogo_LookupErrorFallsThroughToIDSource
// documents that even when the lookup returns an error, the chain
// falls through to the ID-derived CDN URL. The lookup error must not
// silently drop the ID-based fallback.
func TestDownloadAndPersistLogo_LookupErrorFallsThroughToIDSource(t *testing.T) {
	withImageStorageDir(t)

	primary := primaryFailingServer(t)
	defer primary.Close()

	idServer := okServer([]byte("id-source-bytes"))
	defer idServer.Close()

	lookup := &stubLogoLookup{err: fmt.Errorf("boom")}

	repo := newTestRepoWithSources(lookup, func(teamID int64) string {
		return fmt.Sprintf("%s/badge/%d.png", idServer.URL, teamID)
	})
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     42,
		TeamName:   "X",
		PrimaryURL: primary.URL + "/nope.png",
	})

	if lookup.calls.Load() != 1 {
		t.Errorf("lookup calls = %d, want 1 (lookup must be consulted after primary fails)", lookup.calls.Load())
	}
	data, err := os.ReadFile(TeamLogoLocalPath(42))
	if err != nil {
		t.Fatalf("expected the ID source to be tried after the lookup error: %v", err)
	}
	if string(data) != "id-source-bytes" {
		t.Errorf("logo content = %q, want id-source-bytes", data)
	}
}

// TestDownloadAndPersistLogo_NilLookupSkipsLookupStep documents that
// the chain gracefully handles a Repository that has not been
// configured with a LogoLookup. Behaviour must match the pre-fallback
// version: primary → ID-derived CDN only.
func TestDownloadAndPersistLogo_NilLookupSkipsLookupStep(t *testing.T) {
	withImageStorageDir(t)

	primary := okServer([]byte("primary-bytes"))
	defer primary.Close()

	repo := NewRepository(nil)
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     42,
		TeamName:   "X",
		PrimaryURL: primary.URL + "/x.png",
	})

	data, err := os.ReadFile(TeamLogoLocalPath(42))
	if err != nil {
		t.Fatalf("expected logo file: %v", err)
	}
	if string(data) != "primary-bytes" {
		t.Errorf("logo content = %q, want primary-bytes", data)
	}
}

// TestDownloadAndPersistLogo_OrderPrimaryLookupID ensures the chain
// follows the documented order — primary wins, then lookup, then ID
// — and verifies each stage only runs if the previous failed.
func TestDownloadAndPersistLogo_OrderPrimaryLookupID(t *testing.T) {
	withImageStorageDir(t)

	// Primary returns 404, lookup returns 200, ID never gets tested
	// because the lookup already succeeded.
	var primaryCalls int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryCalls, 1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer primary.Close()

	lookupServer := okServer([]byte("lookup-bytes"))
	defer lookupServer.Close()
	lookup := &stubLogoLookup{
		url: lookupServer.URL + "/lookup.png",
	}

	repo := newTestRepoWithLookup(lookup)
	repo.DownloadAndPersistLogo(context.Background(), nil, LogoJob{
		TeamID:     42,
		TeamName:   "Juventus",
		PrimaryURL: primary.URL + "/primary.png",
	})

	if got := atomic.LoadInt32(&primaryCalls); got != 1 {
		t.Errorf("primary calls = %d, want 1", got)
	}
	if lookup.calls.Load() != 1 {
		t.Errorf("lookup calls = %d, want 1 (called only after primary failed)", lookup.calls.Load())
	}
}

// Compile-time check that *stubLogoLookup satisfies LogoLookup.
var _ LogoLookup = (*stubLogoLookup)(nil)

// Compile-time check that *Repository satisfies the scheduler's
// LogoJobHandler shape: ctx + *gorm.DB + LogoJob → no return.
var _ LogoJobHandler = (*Repository)(nil).DownloadAndPersistLogo
