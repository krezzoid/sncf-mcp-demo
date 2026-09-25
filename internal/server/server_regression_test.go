package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNavitiaTimeoutAllowsNormalJourneyLatency(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"journeys":[]}`))
	}))
	defer upstream.Close()

	client := &http.Client{Timeout: navitiaTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { <-started }()
	_, err = client.Do(req)
	if err != nil {
		t.Fatalf("Navitia request failed at %s timeout, want response after normal 2s journey latency: %v", navitiaTimeout, err)
	}
}