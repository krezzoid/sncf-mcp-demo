package transform

import "testing"

func TestNextSummaryEmptyDepartures(t *testing.T) {
	got := NextSummary(nil)
	if got != "No upcoming departures" {
		t.Fatalf("NextSummary(nil) = %q, want %q", got, "No upcoming departures")
	}
}
