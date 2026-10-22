package buildinfo

// Build-time variables injected via -ldflags
// "-X github.com/jeriveromartinez/scores365-scrapper/internal/buildinfo.Version=...
//
//	-X .../internal/buildinfo.Commit=...".
//
// The defaults identify a development build (no -ldflags supplied).
var (
	// Version is the last semver tag baked into the binary (e.g. "v0.0.4").
	Version = "dev"
	// Commit is the short SHA of the commit baked into the binary (e.g. "a0db9ad").
	Commit = "unknown"
	// BuiltAt is reserved for the RFC3339 timestamp of the build. No build
	// script injects it, so it keeps its default value; it is not surfaced
	// in the BuildInfo endpoint.
	BuiltAt = "unknown"
)
