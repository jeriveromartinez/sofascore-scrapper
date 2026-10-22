package apk

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jeriveromartinez/scores365-scrapper/internal/server"
)

// ValidationError is a failure caused by the request itself. Its message
// tells the client what to change, so it can be returned verbatim.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func validationErrorf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// respondClientError returns the validation message when the failure was
// caused by the request and a fixed message otherwise. Database and
// infrastructure details are logged, never sent to the client.
func respondClientError(c *gin.Context, status int, fallback string, err error) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		server.RespondError(c, status, validationErr.Message)
		return
	}

	slog.Default().ErrorContext(c.Request.Context(), "apk: request failed",
		slog.String("path", c.FullPath()),
		slog.String("err", err.Error()))
	server.RespondError(c, status, fallback)
}
