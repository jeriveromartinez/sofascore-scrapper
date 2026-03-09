package httpcli

const (
	FOOTBALL = "football"
	BASKETBALL = "basketball"
	TENNIS = "tennis"
	BASEBALL = "baseball"
	TABLE_TENNIS = "table-tennis"
	VOLLEYBALL = "volleyball"
	RUGBY = "rugby"
)

var sports = []string{
    FOOTBALL,
    BASKETBALL,
    TENNIS,
    BASEBALL,
    TABLE_TENNIS,
    VOLLEYBALL,
    RUGBY,
}

func GET_SPORTS() []string {
    out := make([]string, len(sports))
    copy(out, sports)
    return out
}