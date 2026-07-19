package apk

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidPackageName = errors.New("invalid package name format")
	ErrPathTraversal      = errors.New("path traversal detected")
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*){2,}$`)

func ValidatePackageName(value string) error {
	if strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.Contains(value, "..") || strings.HasPrefix(value, ".") {
		return ErrPathTraversal
	}
	if !packageNamePattern.MatchString(value) {
		return ErrInvalidPackageName
	}
	return nil
}
