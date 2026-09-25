package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

// TestJourneys_EmptyStopDateTimes guards against the panic at line 65
// when a public_transport section has no StopDateTimes entries.
func TestJourneys_EmptyStopDateTimes(t *testing.T) {
	resp := &navitia.JourneysResponse{
		Journeys: []navitia.Journey{
			{
				DurationSeconds:   3600,
				NbTransfers:       0,
				DepartureDateTime: "20260620T140000",
				ArrivalDateTime:   "20260620T150000",
				Sections: []navitia.Section{
					{
						Type:              "public_transport",
						DepartureDateTime: "20260620T140000",
						ArrivalDateTime:   "20260620T150000",
						From:              &navitia.Endpoint{Name: "Paris Gare de Lyon"},
						To:                &navitia.Endpoint{Name: "Lyon Part Dieu"},
						DisplayInfo: &navitia.DisplayInformations{
							CommercialMode: "TGV INOUI",
							Headsign:       "6607",
						},
						// Empty StopDateTimes should not panic; the leg should be skipped
						StopDateTimes: []navitia.StopDateTime{},
					},
				},
			},
		},
	}

	got := Journeys(resp)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// Journey is returned but with 0 legs (the section is skipped)
	if len(got[0].Legs) != 0 {
		t.Errorf("len(Legs) = %d, want 0 (empty StopDateTimes should skip the leg)", len(got[0].Legs))
	}
}
