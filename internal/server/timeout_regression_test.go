package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
)

func TestNavitiaTimeoutAllowsJourneyResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/places" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"places":[{"id":"stop_area:X","name":"X","embedded_type":"stop_area","quality":90}]}`))
			return
		}
		if r.URL.Path == "/journeys" {
			time.Sleep(1600 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"journeys":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	client := navitia.New("k", navitia.WithBaseURL(upstream.URL), navitia.WithRetry(0, 0), navitia.WithHTTPClient(&http.Client{Timeout: navitiaTimeout}))
	_, err := client.Journeys(t.Context(), "stop_area:LYO", "stop_area:LIL", time.Time{}, false)
	if err != nil {
		t.Fatalf("journey request failed for 1600ms response: %v", err)
	}
}
