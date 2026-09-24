package transform

import "testing"

func TestNextSummaryEmpty(t *testing.T) {
    if got := NextSummary([]LeanDeparture{}); got != "" {
        t.Fatalf("Expected empty string, got %q", got)
    }
}