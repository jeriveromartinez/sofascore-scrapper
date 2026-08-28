package users

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MinPasswordLength is the minimum acceptable plaintext length
// (after trimming). NIST 800-63B and OWASP both recommend
// ≥ 12 characters as a starting point; the actual entropy check
// is delegated to the common-password list and any future
// breach-list integration.
const MinPasswordLength = 12

// commonPasswords is a small embedded list of passwords that are
// trivially guessable and appear in every credential-stuffing
// dictionary. Membership is checked case-insensitively after trim.
// This is intentionally small — a full breach-list integration is
// out of scope.
var commonPasswords = map[string]struct{}{
	"password":      {},
	"qwerty":        {},
	"letmein":       {},
	"welcome":       {},
	"iloveyou":      {},
	"admin":         {},
	"changeme":      {},
	"dragon":        {},
	"monkey":        {},
	"football":      {},
	"baseball":      {},
	"superman":      {},
	"batman":        {},
	"trustno1":      {},
	"sunshine":      {},
	"princess":      {},
	"master":        {},
	"shadow":        {},
	"123456789012":  {},
	"qwerty123456":  {},
	"letmein12345":  {},
	"welcome12345":  {},
	"iloveyou12345": {},
	"admin1234567":  {},
	"changeme12345": {},
	"password1234":  {},
}

// ValidatePassword returns nil if the plaintext password is
// acceptable for storage, or a descriptive error otherwise. The
// caller (register / admin create / admin update) MUST reject
// requests with a non-nil result.
//
// Rules enforced:
//   - trim surrounding whitespace
//   - length >= MinPasswordLength after trim
//   - not in the common-password list (case-insensitive)
//
// Future work: NFC normalization for Unicode passwords, full
// breach-list integration.
func ValidatePassword(password string) error {
	trimmed := strings.TrimSpace(password)
	count := utf8.RuneCountInString(trimmed)
	if count < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters (got %d)", MinPasswordLength, count)
	}
	if _, isCommon := commonPasswords[strings.ToLower(trimmed)]; isCommon {
		return errors.New("password is too common; choose a different one")
	}
	return nil
}
