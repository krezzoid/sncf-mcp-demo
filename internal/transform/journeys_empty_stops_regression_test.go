package transform

import (
	"testing"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestJourneys_EmptyStopDateTimesDoesNotPanic(t *testing.T) {
	resp := &navitia.JourneysResponse{Journeys: []navitia.Journey{{Sections: []navitia.Section{{
		Type: "public_transport",
		DisplayInfo: &navitia.DisplayInformations{CommercialMode: "TGV INOUI", Headsign: "6607"},
		StopDateTimes: []navitia.StopDateTime{},
	}}}}}
	got := Journeys(resp)
	if len(got) != 1 || len(got[0].Legs) != 1 {
		t.Fatalf("Journeys result = %+v, want one journey with one leg", got)
	}
	if got[0].Legs[0].DelayMin != 0 {
		t.Fatalf("DelayMin = %d, want 0 when stop date-times are absent", got[0].Legs[0].DelayMin)
	}
}
