package scores365

// statusMap maps 365scores GT (game-time) codes to a free-form
// description used in MatchStatus.Description. The bool-flag
// translation that the events repo queries against
// (Started/Finished/Cancelled) lives in statusFlags in source.go.
//
// Codes not in the map fall back to a Completion/ETime/STime
// heuristic in statusFlags.
var statusMap = map[int]string{
	-1: "notstarted",
	1:  "scheduled",
	12: "live", // first half
	13: "live", // second half
	22: "live", // first quarter
	23: "live", // second quarter
	24: "live", // third quarter
	25: "live", // fourth quarter
	26: "live", // first inning (baseball)
	75: "live", // 1st inning
	76: "live", // 2nd inning
	77: "live", // 3rd inning
	78: "live", // 4th inning
	79: "live", // 5th inning
	80: "live", // 6th inning (MLB live)
	3:  "finished",
	32: "finished",
	33: "finished",
	88: "finished",
	4:  "postponed",
	5:  "cancelled",
	8:  "interrupted",
	9:  "walkover",
}

// sportMap converts 365scores SID to canonical sport slug.
var sportMap = map[int]string{
	1: "football",
	2: "basketball",
	3: "tennis",
	4: "ice-hockey",
	6: "american-football",
	7: "baseball",
	8: "volleyball",
}

func sportSlugFromSID(sid int) string {
	if s, ok := sportMap[sid]; ok {
		return s
	}
	return "unknown"
}
