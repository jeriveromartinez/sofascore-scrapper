package buildinfo

// Build-time variables injected via -ldflags
// "-X github.com/jeriveromartinez/sofascore-scrapper/internal/buildinfo.Version=...
//  -X .../internal/buildinfo.Commit=...
//  -X .../internal/buildinfo.BuiltAt=...".
// The defaults identify a development build (no -ldflags supplied).
var (
	// Version is the last semver tag baked into the binary (e.g. "v0.0.4").
	Version = "dev"
	// Commit is the short SHA of the commit baked into the binary (e.g. "a0db9ad").
	Commit = "unknown"
	// BuiltAt is the RFC3339 timestamp when the binary was built. Exposed for
	// future log/debug use; not currently surfaced in the BuildInfo endpoint.
	BuiltAt = "unknown"
)
