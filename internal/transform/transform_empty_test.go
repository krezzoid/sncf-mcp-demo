package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneys_EmptyStopDateTimes_DoesNotPanic(t *testing.T) {
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
						From:              &navitia.Endpoint{Name: "A"},
						To:                &navitia.Endpoint{Name: "B"},
						DisplayInfo: &navitia.DisplayInformations{
							CommercialMode: "TER",
							Headsign:       "12345",
						},
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
	// Should not panic, just have no legs
	if len(got[0].Legs) != 0 {
		t.Errorf("Legs = %d, want 0 (empty StopDateTimes should be skipped)", len(got[0].Legs))
	}
}
