package realtime

import "encoding/base64"

// encodeBase64 / decodeBase64 are thin wrappers kept in their own
// file so the rest of the package does not need to import
// encoding/base64 directly. Centralized for easy swap to a different
// encoding (e.g. raw) if we ever want to shrink the Redis payload.
func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
