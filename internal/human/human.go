// Package human provides small formatting helpers for user-facing output.
package human

import "fmt"

// Bytes renders a byte count in a compact, human-readable form.
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Plural returns "s" unless n is 1.
func Plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
