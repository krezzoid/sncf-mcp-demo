package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneysEmptyStopDateTimes(t *testing.T) {
	resp := &navitia.JourneysResponse{Journeys: []navitia.Journey{{
		Sections: []navitia.Section{{
			Type:          "public_transport",
			DisplayInfo:   &navitia.DisplayInformations{},
			StopDateTimes: []navitia.StopDateTime{},
		}},
	}}}
	got := Journeys(resp)
	if len(got) != 1 || len(got[0].Legs) != 1 || got[0].Legs[0].DelayMin != 0 {
		t.Fatalf("Journeys() with an empty StopDateTimes should retain the leg with zero delay; got %#v", got)
	}
}
