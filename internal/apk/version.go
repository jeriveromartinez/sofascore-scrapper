package apk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func IsNewerVersion(current, candidate string) (bool, error) {
	cur, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	can, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("invalid candidate version %q: %w", candidate, err)
	}
	for i := range cur {
		if can[i] > cur[i] {
			return true, nil
		}
		if can[i] < cur[i] {
			return false, nil
		}
	}
	return false, nil
}

func ParseSemverComponents(v string) (major, minor, patch uint64, err error) {
	parts, err := parseVersion(v)
	if err != nil {
		return 0, 0, 0, err
	}
	return uint64(parts[0]), uint64(parts[1]), uint64(parts[2]), nil
}

func parseVersion(v string) ([3]int, error) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, errors.New("version must have exactly 3 components (MAJOR.MINOR.PATCH)")
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("component %d is not a non-negative integer", i)
		}
		result[i] = n
	}
	return result, nil
}
