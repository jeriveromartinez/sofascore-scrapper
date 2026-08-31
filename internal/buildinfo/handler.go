// Package buildinfo exposes the version/commit baked into the binary
// at compile time. The package-level variables Version, Commit, and
// BuiltAt are resolved via -ldflags at build time (see Dockerfile).
// The defaults identify a dev build (no -ldflags).
package buildinfo

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"google.golang.org/protobuf/proto"
)

// Handler serves GET /version (mounted under the webV1 group in
// internal/app/routes.go so the full path becomes /api/web/v1/version).
type Handler struct {
	logger *slog.Logger
}

// Register installs the route on the provided router.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/version", h.Handle)
}

// Handle responds with the BuildInfo for the running binary.
func (h *Handler) Handle(c *gin.Context) {
	body, err := proto.Marshal(&pb.BuildInfo{
		Version: Version,
		Commit:  Commit,
	})
	if err != nil {
		h.logger.Error("buildinfo: marshal failed",
			slog.String("error", err.Error()))
		c.String(http.StatusInternalServerError, "marshal failed")
		return
	}
	c.Data(http.StatusOK, "application/x-protobuf", body)
}
