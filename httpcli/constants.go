package httpcli

const (
	FOOTBALL     = "football"
	BASKETBALL   = "basketball"
	TENNIS       = "tennis"
	BASEBALL     = "baseball"
	TABLE_TENNIS = "table-tennis"
	VOLLEYBALL   = "volleyball"
	RUGBY        = "rugby"
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

var countries = []string{
	"MX", "AR", "CL", "CO", "PE", "VE", "UY", "EC", "BO", "PY", "CR", "PA", "DO", "GT", "HN", "NI", "SV", "US", "CA", "BR", "GB", "DE", "FR", "IT", "ES", "RU", "CN", "JP", "KR", "IN", "AU", "ZA",
}

func GET_SPORTS() []string {
	out := make([]string, len(sports))
	copy(out, sports)
	return out
}

func GET_COUNTRIES() []string {
	out := make([]string, len(countries))
	copy(out, countries)
	return out
}
