package config

import (
	"testing"
)

func TestRankingModeString(t *testing.T) {
	tests := []struct {
		mode     RankingMode
		expected string
	}{
		{RankingDailyMode, "daily"},
		{RankingWeeklyMode, "weekly"},
		{RankingMonthlyMode, "monthly"},
		{RankingMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("RankingMode.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
