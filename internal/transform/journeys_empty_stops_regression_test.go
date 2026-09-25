package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneys_EmptyStopDateTimesDoesNotPanic(t *testing.T) {
	resp := &navitia.JourneysResponse{
		Journeys: []navitia.Journey{{
			Sections: []navitia.Section{{
				Type: "public_transport",
				DisplayInfo: &navitia.DisplayInformations{CommercialMode: "TGV INOUI"},
				DepartureDateTime: "20260620T140000",
				ArrivalDateTime: "20260620T155600",
			}},
		}},
	}
	got := Journeys(resp)
	if len(got) != 1 || len(got[0].Legs) != 1 {
		t.Fatalf("Journeys returned %+v; want one journey with one leg even when stop_date_times is empty", got)
	}
	if got[0].Legs[0].DelayMin != 0 {
		t.Fatalf("DelayMin = %d, want 0 when no boarding stop date-time is available", got[0].Legs[0].DelayMin)
	}
}
