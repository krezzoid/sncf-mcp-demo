package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHTTP_StatelessToolCall checks the contract the demo stack relies on: a
// single POST with tools/call, no initialize handshake and no session header,
// gets a plain JSON answer, and the call shows up in /metrics and the log.
func TestHTTP_StatelessToolCall(t *testing.T) {
	navitia := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/places"):
			_, _ = w.Write([]byte(`{"places":[{"id":"stop_area:X","name":"X","embedded_type":"stop_area","quality":90}]}`))
		case strings.HasSuffix(r.URL.Path, "/departures"):
			_, _ = w.Write([]byte(`{"departures":[{"display_informations":{"commercial_mode":"TER","direction":"Lyon Perrache (Lyon)","headsign":"96521"},` +
				`"stop_date_time":{"departure_date_time":"20260618T143300","base_departure_date_time":"20260618T143000","data_freshness":"realtime"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer navitia.Close()

	var logs bytes.Buffer
	cfg := Config{APIKey: "k", BaseURL: navitia.URL, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	cfg.defaults()
	srv := httptest.NewServer(httpHandler(cfg, build(cfg)))
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"next_departures","arguments":{"station":"X"}}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Request-Id", "replay-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("status %d, content-type %q, body %s", resp.StatusCode, resp.Header.Get("Content-Type"), raw)
	}
	var rpc struct {
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	var out struct {
		Departures []struct {
			Train string `json:"train"`
		} `json:"departures"`
	}
	if err := json.Unmarshal(raw, &rpc); err != nil || rpc.Result.IsError || json.Unmarshal(rpc.Result.StructuredContent, &out) != nil ||
		len(out.Departures) != 1 || out.Departures[0].Train != "96521" {
		t.Fatalf("unexpected response: %s", raw)
	}
	if !strings.Contains(logs.String(), `"request_id":"replay-1"`) {
		t.Errorf("request id from header not logged: %s", logs.String())
	}

	metrics, _ := http.Get(srv.URL + "/metrics")
	text, _ := io.ReadAll(metrics.Body)
	_ = metrics.Body.Close()
	for _, want := range []string{
		`sncf_mcp_tool_calls_total{outcome="ok",tool="next_departures"} 1`,
		`sncf_mcp_upstream_requests_total{code="200",endpoint="departures"} 1`,
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("/metrics missing %s", want)
		}
	}
}
