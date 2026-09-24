package transform

import "testing"

func TestNextSummary_ReturnsEmptyOnEmptySlice(t *testing.T) {
	deps := []LeanDeparture{}
	if got := NextSummary(deps); got != "" {
		t.Errorf("NextSummary on empty slice = %q, want empty string", got)
	}
}