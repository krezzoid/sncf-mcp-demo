package transform

import "testing"

func TestNextSummaryEmptySlice(t *testing.T) {
	deps := []LeanDeparture{}
	got := NextSummary(deps)
	if got != "" {
		t.Errorf("NextSummary([]LeanDeparture{}) = %q; want empty string", got)
	}
}