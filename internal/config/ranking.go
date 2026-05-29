package config

type RankingMode int

const (
	RankingDailyMode RankingMode = iota
	RankingWeeklyMode
	RankingMonthlyMode
)

// String returns the string representation of a ranking mode.
func (r RankingMode) String() string {
	switch r {
	case RankingDailyMode:
		return "daily"
	case RankingWeeklyMode:
		return "weekly"
	case RankingMonthlyMode:
		return "monthly"
	default:
		return "unknown"
	}
}
