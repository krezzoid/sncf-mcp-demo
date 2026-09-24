package transform

import "testing"

func TestNextSummary_Empty(t *testing.T) {
	if got := NextSummary([]LeanDeparture{}); got != "" {
		t.Errorf("NextSummary on empty slice = %q, want empty string", got)
	}
}
