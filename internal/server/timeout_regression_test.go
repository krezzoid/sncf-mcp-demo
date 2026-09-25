package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNavitiaTimeoutAllowsSlowJourneyResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/coverage/sncf/journeys" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"journeys":[]}`))
	}))
	defer upstream.Close()

	client := &http.Client{Timeout: navitiaTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL+"/v1/coverage/sncf/journeys", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("slow Navitia response should be allowed; got request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}
