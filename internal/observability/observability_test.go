package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type stationInput struct {
	Station string `json:"station"`
}

type empty struct{}

// session starts an in-memory MCP server with the middleware and one tool per
// outcome, and returns a connected client session plus the captured log.
func session(t *testing.T) (*mcp.ClientSession, *Metrics, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	m := NewMetrics("test", "abc123")

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	srv.AddReceivingMiddleware(ToolCalls(logger, m))
	mcp.AddTool(srv, &mcp.Tool{Name: "ok"}, func(context.Context, *mcp.CallToolRequest, stationInput) (*mcp.CallToolResult, empty, error) {
		return nil, empty{}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "unresolved"}, func(context.Context, *mcp.CallToolRequest, stationInput) (*mcp.CallToolResult, empty, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "no such station"}}}, empty{}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "upstream_down"}, func(context.Context, *mcp.CallToolRequest, stationInput) (*mcp.CallToolResult, empty, error) {
		return nil, empty{}, errors.New("navitia: unexpected status 503")
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "nil_deref"}, func(_ context.Context, _ *mcp.CallToolRequest, in stationInput) (*mcp.CallToolResult, empty, error) {
		var departures []string
		_ = departures[len(in.Station)] // index out of range
		return nil, empty{}, nil
	})

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, m, &logs
}

func call(t *testing.T, cs *mcp.ClientSession, tool string) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"station": "Lyon Part Dieu"}})
}

// lastLog decodes the last JSON log line.
func lastLog(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("decode log line %q: %v", lines[len(lines)-1], err)
	}
	return entry
}

func TestToolCalls_Outcomes(t *testing.T) {
	cs, m, logs := session(t)

	cases := []struct {
		tool, outcome, level string
		wantErr              bool
	}{
		{"ok", OutcomeOK, "INFO", false},
		{"unresolved", OutcomeToolError, "INFO", false},
		{"upstream_down", OutcomeError, "ERROR", false}, // the SDK turns handler errors into IsError results
		{"nil_deref", OutcomePanic, "ERROR", true},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			_, err := call(t, cs, c.tool)
			if (err != nil) != c.wantErr {
				t.Fatalf("CallTool err = %v, wantErr %v", err, c.wantErr)
			}
			if got := testutil.ToFloat64(m.toolCalls.WithLabelValues(c.tool, c.outcome)); got != 1 {
				t.Errorf("sncf_mcp_tool_calls_total{tool=%q,outcome=%q} = %v, want 1", c.tool, c.outcome, got)
			}
			entry := lastLog(t, logs)
			if entry["msg"] != "tool call" || entry["tool"] != c.tool || entry["outcome"] != c.outcome || entry["level"] != c.level {
				t.Errorf("log = %v", entry)
			}
			if args, _ := entry["args"].(map[string]any); args["station"] != "Lyon Part Dieu" {
				t.Errorf("args not logged as JSON: %v", entry["args"])
			}
			if id, _ := entry["request_id"].(string); !validRequestID.MatchString(id) {
				t.Errorf("request_id = %q", id)
			}
		})
	}
}

func TestToolCalls_InvalidArgumentsAreToolErrors(t *testing.T) {
	cs, m, _ := session(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "ok", Arguments: map[string]any{"station": 42}})
	if err != nil || !res.IsError {
		t.Fatalf("res = %+v, err = %v; want an IsError result", res, err)
	}
	if got := testutil.ToFloat64(m.toolCalls.WithLabelValues("ok", OutcomeToolError)); got != 1 {
		t.Errorf("invalid arguments counted as %v tool_error, want 1", got)
	}
}

func TestToolCalls_PanicIsIsolated(t *testing.T) {
	cs, _, logs := session(t)
	if _, err := call(t, cs, "nil_deref"); err == nil || !strings.Contains(err.Error(), "internal error") {
		t.Fatalf("err = %v, want internal error", err)
	}
	entry := lastLog(t, logs)
	if !strings.Contains(entry["panic"].(string), "index out of range") {
		t.Errorf("panic = %v", entry["panic"])
	}
	if !strings.Contains(entry["stack"].(string), "observability_test.go") {
		t.Errorf("stack does not point at the handler: %v", entry["stack"])
	}
	// The server keeps serving after a panic.
	if _, err := call(t, cs, "ok"); err != nil {
		t.Fatalf("call after panic: %v", err)
	}
}

func TestRequestIDFromHeader(t *testing.T) {
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: http.Header{RequestIDHeader: {"replay-42"}}}}
	if got := requestIDFrom(req); got != "replay-42" {
		t.Errorf("requestIDFrom = %q, want replay-42", got)
	}
	req.Extra.Header.Set(RequestIDHeader, "bad id\nwith newline")
	if got := requestIDFrom(req); got == "bad id\nwith newline" || !validRequestID.MatchString(got) {
		t.Errorf("unsafe header accepted: %q", got)
	}
}

func TestClientAttrs(t *testing.T) {
	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: http.Header{
		ForwardedForHeader:   {"203.0.113.24, 10.0.3.2"},
		ForwardedEmailHeader: {"manon.petit@example.fr"},
		APIKeyHeader:         {"sncf_live_0123456789abcdef"},
	}}}
	got := fmt.Sprint(clientAttrs(req))
	if want := "[client_ip 203.0.113.24 user manon.petit@example.fr api_key sncf_live_0123456789abcdef]"; got != want {
		t.Errorf("clientAttrs = %s, want %s", got, want)
	}

	// A forged or garbled address is not logged; a call without the gateway logs no client.
	req.Extra.Header = http.Header{ForwardedForHeader: {"not-an-ip\n{\"level\":\"ERROR\"}"}}
	if got := clientAttrs(req); len(got) != 0 {
		t.Errorf("clientAttrs = %v, want none", got)
	}
	if got := clientAttrs(&mcp.CallToolRequest{}); len(got) != 0 {
		t.Errorf("clientAttrs without headers = %v", got)
	}
}

func TestLoggedArgs(t *testing.T) {
	if got := loggedArgs(nil); string(got.(json.RawMessage)) != "{}" {
		t.Errorf("empty args = %v", got)
	}
	long := json.RawMessage(`{"station":"` + strings.Repeat("x", maxLoggedArgs) + `"}`)
	if got, ok := loggedArgs(long).(string); !ok || len(got) > maxLoggedArgs+len("…") {
		t.Errorf("oversized args not truncated: %T len %d", got, len(got))
	}
}

func TestEndpointLabel(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/coverage/sncf/places":                                 "places",
		"/v1/coverage/sncf/journeys":                               "journeys",
		"/v1/coverage/sncf/stop_areas/stop_area:SNCF:1/departures": "departures",
		"/v1/coverage/sncf/disruptions":                            "disruptions",
		"/v1/coverage/sncf/stop_areas/x/disruptions":               "disruptions",
		"/v1/coverage/sncf/lines":                                  "other",
	} {
		if got := EndpointLabel(path); got != want {
			t.Errorf("EndpointLabel(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestUpstreamTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/departures") {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	m := NewMetrics("test", "abc123")
	client := &http.Client{Transport: UpstreamTransport(http.DefaultTransport, m, slog.New(slog.NewJSONHandler(&logs, nil)))}
	ctx := context.WithValue(context.Background(), requestIDKey{}, "req-7")
	for _, path := range []string{"/places", "/stop_areas/x/departures"} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL+path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}

	if got := testutil.ToFloat64(m.upstreamRequests.WithLabelValues("places", "200")); got != 1 {
		t.Errorf("places 200 = %v", got)
	}
	if got := testutil.ToFloat64(m.upstreamRequests.WithLabelValues("departures", "503")); got != 1 {
		t.Errorf("departures 503 = %v", got)
	}
	entry := lastLog(t, &logs)
	if entry["msg"] != "upstream request failed" || entry["endpoint"] != "departures" || entry["request_id"] != "req-7" {
		t.Errorf("log = %v", entry)
	}
	if strings.Count(strings.TrimSpace(logs.String()), "\n") != 0 {
		t.Errorf("successful upstream calls must not be logged: %s", logs.String())
	}
}

func TestBuildInfo(t *testing.T) {
	m := NewMetrics("v1.2.3", "deadbee")
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `sncf_mcp_build_info{commit="deadbee",version="v1.2.3"} 1`) {
		t.Errorf("build info missing from /metrics")
	}
}
