package server

import "strconv"

func ParseID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
