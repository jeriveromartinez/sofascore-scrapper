package buildinfo

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

// withVersionAndCommit overrides the package globals for the
// duration of one test, then restores them.
func withVersionAndCommit(t *testing.T, v, c string) {
	t.Helper()
	prevVersion, prevCommit := Version, Commit
	Version, Commit = v, c
	t.Cleanup(func() { Version, Commit = prevVersion, prevCommit })
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&Handler{logger: slog.Default()}).Register(r)
	return r
}

func TestHandle_ReturnsDefaults(t *testing.T) {
	// Version/Commit default to "dev" / "unknown".
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	got := &pb.BuildInfo{}
	if err := proto.Unmarshal(w.Body.Bytes(), got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != "dev" {
		t.Errorf("Version = %q, want %q", got.Version, "dev")
	}
	if got.Commit != "unknown" {
		t.Errorf("Commit = %q, want %q", got.Commit, "unknown")
	}
}

func TestHandle_ReturnsOverride(t *testing.T) {
	withVersionAndCommit(t, "v0.0.4", "a0db9ad")
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := &pb.BuildInfo{}
	if err := proto.Unmarshal(w.Body.Bytes(), got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != "v0.0.4" {
		t.Errorf("Version = %q, want %q", got.Version, "v0.0.4")
	}
	if got.Commit != "a0db9ad" {
		t.Errorf("Commit = %q, want %q", got.Commit, "a0db9ad")
	}
}
