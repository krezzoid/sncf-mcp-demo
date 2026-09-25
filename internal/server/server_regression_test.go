package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
	"github.com/krezzoid/sncf-mcp/internal/tools"
)

func TestPlanJourneySlowNavitiaTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/places"):
			name := r.URL.Query().Get("q")
			id := "stop_area:SNCF:87751008"
			if strings.Contains(name, "Bordeaux") {
				id = "stop_area:SNCF:87581009"
			}
			_, _ = w.Write([]byte(`{"places":[{"id":"` + id + `","name":"` + name + `","embedded_type":"stop_area","quality":90}]}`))
		case strings.HasSuffix(r.URL.Path, "/journeys"):
			time.Sleep(2 * time.Second)
			_, _ = w.Write([]byte(`{"journeys":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	client := navitia.New("test-key", navitia.WithBaseURL(upstream.URL), navitia.WithHTTPClient(&http.Client{Timeout: navitiaTimeout}))
	_, _, err := tools.New(client).PlanJourney(context.Background(), nil, tools.PlanJourneyInput{From: "Marseille Saint-Charles", To: "Bordeaux Saint-Jean"})
	if err != nil {
		if !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("PlanJourney returned unrelated error: %v", err)
		}
		t.Fatalf("plan_journey Navitia request failed: %v", err)
	}
}
