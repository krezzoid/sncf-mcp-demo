package transform

import "testing"

func TestNextSummaryEmpty(t *testing.T) {
    deps := []LeanDeparture{}
    if got := NextSummary(deps); got != "" {
        t.Fatalf(`NextSummary(empty) = %q, want ""`, got)
    }
}
