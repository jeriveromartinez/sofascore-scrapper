package server

import "strconv"

// ParseID parses a decimal row id coming from a URL path parameter or
// from a decoded pagination cursor.
//
// The bit size is 0, which strconv maps to the width of `uint` — the
// type gorm.Model uses for primary keys. It used to be 32, which was
// fine while every id was a MariaDB auto-increment counter, but the
// scraper now namespaces upstream ids into their own ranges
// (scores365.LeagueIDPrefix = 6e9 for tournaments/leagues,
// events.TeamIDPrefix = 7e9 for teams). Those overflow uint32, so a
// 32-bit parse rejected legitimate rows: paging past a tournament with
// a 6e9 id failed with "invalid cursor: bad id", and GET/PUT/DELETE
// /tournaments/:id returned 400 for the same rows.
func ParseID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 0)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
