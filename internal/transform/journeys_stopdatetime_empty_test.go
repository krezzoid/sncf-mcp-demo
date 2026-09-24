package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneysEmptyStopDateTimes(t *testing.T) {
	// Create a JourneysResponse with one Journey that has a Section
	// with public_transport, DisplayInfo not nil, but empty StopDateTimes.
	resp := &navitia.JourneysResponse{
		Journeys: []navitia.Journey{
			{
				DepartureDateTime: "20260924T100000",
				ArrivalDateTime:   "20260924T120000",
				DurationSeconds:   7200, // 2 hours
				NbTransfers:       0,
				Sections: []navitia.Section{
					{
						Type:       "public_transport",
						DisplayInfo: &navitia.DisplayInformations{
							CommercialMode: "TGV",
							Headsign:       "Paris Lyon",
						},
						// StopDateTimes is empty, simulating missing real-time data
						StopDateTimes: []navitia.StopDateTime{},
						From: &navitia.Endpoint{
							Name: "Paris Gare de Lyon",
						},
						To: &navitia.Endpoint{
							Name: "Lyon Part-Dieu",
						},
					},
				},
			},
		},
	}

	// This should not panic.
	result := Journeys(resp)
	if result == nil || len(result) == 0 {
		 t.Fatalf("Expected at least one journey")
	}
	// We can also check that the leg has DelayMin 0 and From/To set.
	if len(result[0].Legs) != 1 {
		 t.Fatalf("Expected exactly one leg")
	}
	leg := result[0].Legs[0]
	if leg.DelayMin != 0 {
		 t.Errorf("Expected DelayMin to be 0, got %d", leg.DelayMin)
	}
	if leg.From != "Paris Gare de Lyon" {
		 t.Errorf("Expected From to be Paris Gare de Lyon, got %s", leg.From)
	}
	if leg.To != "Lyon Part-Dieu" {
		 t.Errorf("Expected To to be Lyon Part-Dieu, got %s", leg.To)
	}
}