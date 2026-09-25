package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneysEmptyStopDateTimes(t *testing.T) {
	resp := &navitia.JourneysResponse{
		Journeys: []navitia.Journey{
			{
				DepartureDateTime: "20260925T200000",
				ArrivalDateTime:   "20260925T220000",
				DurationSeconds:   7200,
				Sections: []navitia.Section{
					{
						Type:          "public_transport",
						DisplayInfo:   &navitia.DisplayInformations{CommercialMode: "TGV INOUI", Headsign: "Paris"},
						StopDateTimes: []navitia.StopDateTime{},
					},
				},
			},
		},
	}

	got := Journeys(resp)
	if len(got) != 1 || len(got[0].Legs) != 1 {
		t.Fatalf("Journeys() should preserve the public-transport leg without stop date-times; got %#v", got)
	}
	if got[0].Legs[0].DelayMin != 0 {
		t.Fatalf("DelayMin = %d, want 0 when stop date-times are unavailable", got[0].Legs[0].DelayMin)
	}
}
