package tiamat

import (
	"fmt"
	"strings"
	"time"
)

const dateOnlyLayout = "2006-01-02"

// FormatHubStatsParam converts a stats query value for Tiamat's hub API.
// Accepts RFC3339 (e.g. 2006-01-02T15:04:05Z07:00) or a calendar date YYYY-MM-DD.
// For date-only input: from uses start of that day UTC; to uses end of that day UTC (23:59:59Z).
// Empty input returns ("", nil).
func FormatHubStatsParam(s string, endOfDay bool) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(dateOnlyLayout, s); err == nil {
		y, m, d := t.Date()
		if endOfDay {
			t = time.Date(y, m, d, 23, 59, 59, 0, time.UTC)
		} else {
			t = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		}
		return t.Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid date %q: use YYYY-MM-DD or RFC3339 (e.g. 2006-01-02T15:04:05Z)", s)
}
