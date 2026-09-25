package transform

import "testing"

func TestNextSummary_EmptySliceReturnsEmptyString(t *testing.T) {
    deps := []LeanDeparture{}
    if got := NextSummary(deps); got != "" {
        t.Errorf("NextSummary([]LeanDeparture{}) = %q; want empty string", got)
    }
}