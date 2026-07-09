package config

import (
	"testing"
)

func TestQueueSourceString(t *testing.T) {
	tests := []struct {
		source   QueueSource
		expected string
	}{
		{QueueSourceRanking, QueueSourceStrRanking},
		{QueueSourceBookmarks, QueueSourceStrBookmarks},
		{QueueSourceAll, QueueSourceStrAll},
		{QueueSource(99), QueueSourceStrRanking},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.source.String(); got != tt.expected {
				t.Errorf("QueueSource.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestQueueSourceDefault(t *testing.T) {
	var s QueueSource
	if s != QueueSourceRanking {
		t.Errorf("default QueueSource = %d, want %d", s, QueueSourceRanking)
	}
}
