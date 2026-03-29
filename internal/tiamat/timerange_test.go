package tiamat

import "testing"

func TestFormatHubStatsParam_dateOnly(t *testing.T) {
	got, err := FormatHubStatsParam("2026-03-01", false)
	if err != nil || got != "2026-03-01T00:00:00Z" {
		t.Fatalf("from: %q %v", got, err)
	}
	got, err = FormatHubStatsParam("2026-03-01", true)
	if err != nil || got != "2026-03-01T23:59:59Z" {
		t.Fatalf("to: %q %v", got, err)
	}
}

func TestFormatHubStatsParam_RFC3339(t *testing.T) {
	got, err := FormatHubStatsParam("2026-03-01T12:30:00-04:00", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-03-01T16:30:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatHubStatsParam_empty(t *testing.T) {
	got, err := FormatHubStatsParam("  ", false)
	if err != nil || got != "" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestFormatHubStatsParam_invalid(t *testing.T) {
	if _, err := FormatHubStatsParam("not-a-date", false); err == nil {
		t.Fatal("expected error")
	}
}
