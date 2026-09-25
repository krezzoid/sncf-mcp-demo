package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneysEmptyStopDateTimes(t *testing.T) {
	resp := &navitia.JourneysResponse{
		Journeys: []navitia.Journey{{
			Sections: []navitia.Section{{
				Type: "public_transport",
				DisplayInfo: &navitia.DisplayInformations{},
				StopDateTimes: nil,
			}},
		}},
	}
	got := Journeys(resp)
	if len(got) != 1 || len(got[0].Legs) != 0 {
		t.Fatalf("Journeys() with no stop date times should keep journey and omit leg; got %#v", got)
	}
}
