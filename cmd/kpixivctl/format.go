package main

import (
	"fmt"
	"time"
	"unicode"

	"github.com/alphonse927/kpixiv/internal/human"
)

// keyValue renders an aligned "Key: value" line. width controls the key
// column so related lines line up.
func keyValue(width int, key string, value any) string {
	return fmt.Sprintf("  %-*s %v", width, key+":", value)
}

// capitalize uppercases the first rune of s.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func formatBytes(n int64) string {
	return human.Bytes(n)
}
