package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
	"github.com/krezzoid/sncf-mcp/internal/observability"
	"github.com/krezzoid/sncf-mcp/internal/tools"
)

func TestPlanJourneyTimeoutReproduction(t *testing.T) {
	// Mock Navitia server that delays the journey response by 2 seconds.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow Navitia by sleeping for 2 seconds.
		time.Sleep(2 * time.Second)
		// Return a valid journey response (minimal).
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"journeys": [{
				"nb_transfers": 0,
				"sections": [{
					"type": "transfer",
					"from": {"stop_area": {"id": "stop_area:SNCF:87286005"}},
					"to": {"stop_area": {"id": "stop_area:SNCF:87581009"}},
					"display_info": {
						"commercial_mode": "TGV",
						"label": "TGV 123"
					}
				}]
			}]
		}`))
	}))
	defer mockServer.Close()

	// Configure the server to point to our mock Navitia.
	cfg := Config{}
	cfg.defaults()
	cfg.BaseURL = mockServer.URL
	cfg.APIKey = "test-key"

	// Override the HTTP client to use the navitiaTimeout from server.go.
	opts := []navitia.Option{
		navitia.WithHTTPClient(&http.Client{
			Timeout:   navitiaTimeout,
			Transport: observability.UpstreamTransport(http.DefaultTransport, cfg.Metrics, cfg.Logger),
		}),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, navitia.WithBaseURL(cfg.BaseURL))
	}

	client := navitia.New(cfg.APIKey, opts...)
	h := tools.New(client)

	// Use a context that is long enough to allow the request to complete (e.g., 10 seconds).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := tools.PlanJourneyInput{
		From: "Lille Flandres",
		To:   "Bordeaux Saint-Jean",
	}

	// Call the PlanJourney tool.
	_, _, err := h.PlanJourney(ctx, nil, in)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}