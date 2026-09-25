package transform

import "testing"

func TestNextSummaryEmptyReturnsEmptyString(t *testing.T) {
    deps := []LeanDeparture{}
    got := NextSummary(deps)
    if got != "" {
        t.Errorf("NextSummary([]LeanDeparture{}) = %q; want empty string", got)
    }
}