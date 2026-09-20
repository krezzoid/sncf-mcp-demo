// Package observability makes the server operable: Prometheus metrics, one
// structured log line per tool call (with the arguments needed to replay it),
// panic isolation for tool handlers, and metrics for upstream Navitia calls.
//
// Everything here sits outside the tool handlers, so the handlers and the
// transform layer stay free of operational concerns.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RequestIDHeader carries a caller-supplied request id; one is generated when absent.
const RequestIDHeader = "X-Request-Id"

// Tool call outcomes, used as the "outcome" metric label and log attribute.
const (
	OutcomeOK        = "ok"         // handler returned a result
	OutcomeToolError = "tool_error" // result with IsError: bad input, unresolved station
	OutcomeError     = "error"      // handler returned an error, e.g. upstream failure
	OutcomePanic     = "panic"      // handler panicked; recovered and turned into an error
)

// maxLoggedArgs bounds the size of logged tool arguments.
const maxLoggedArgs = 2048

// Metrics holds the server's Prometheus collectors on a private registry.
type Metrics struct {
	registry         *prometheus.Registry
	toolCalls        *prometheus.CounterVec
	toolDuration     *prometheus.HistogramVec
	upstreamRequests *prometheus.CounterVec
	upstreamDuration *prometheus.HistogramVec
}

// NewMetrics registers the server metrics. version and commit are exported as
// sncf_mcp_build_info so a change in behavior can be matched to a deploy.
func NewMetrics(version, commit string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	build := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "sncf_mcp_build_info",
		Help:        "Build of the running server; always 1.",
		ConstLabels: prometheus.Labels{"version": version, "commit": commit},
	})
	build.Set(1)

	m := &Metrics{
		registry: reg,
		toolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sncf_mcp_tool_calls_total",
			Help: "MCP tool calls by tool and outcome (ok, tool_error, error, panic).",
		}, []string{"tool", "outcome"}),
		toolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "sncf_mcp_tool_call_duration_seconds",
			Help:    "MCP tool call latency.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"tool"}),
		upstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sncf_mcp_upstream_requests_total",
			Help: "Requests to the Navitia API by endpoint and HTTP status (\"error\" for transport failures).",
		}, []string{"endpoint", "code"}),
		upstreamDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "sncf_mcp_upstream_request_duration_seconds",
			Help:    "Navitia API request latency.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"endpoint"}),
	}
	reg.MustRegister(build, m.toolCalls, m.toolDuration, m.upstreamRequests, m.upstreamDuration)
	return m
}

// Handler serves the metrics in the Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Registry exposes the registry, e.g. for tests.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

type requestIDKey struct{}

// RequestID returns the request id stored in ctx by the tool-call middleware.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// ToolCalls returns MCP middleware that, for every tools/call, assigns a request
// id, recovers a panicking handler, records metrics and writes one log line with
// the tool name and its arguments, so a failing call can be found and replayed.
func ToolCalls(logger *slog.Logger, m *Metrics) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			tool, args := "unknown", json.RawMessage(nil)
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && p != nil {
				tool, args = p.Name, p.Arguments
			}
			id := requestIDFrom(req)
			ctx = context.WithValue(ctx, requestIDKey{}, id)
			start := time.Now()

			defer func() {
				outcome := OutcomeOK
				attrs := []any{"request_id", id, "tool", tool, "args", loggedArgs(args)}
				if p := recover(); p != nil {
					outcome = OutcomePanic
					result, err = nil, fmt.Errorf("internal error while running tool %q (request_id %s)", tool, id)
					attrs = append(attrs, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
				} else if err != nil {
					outcome = OutcomeError
					attrs = append(attrs, "error", err.Error())
				} else if r, ok := result.(*mcp.CallToolResult); ok && r != nil && r.IsError {
					// The SDK turns a handler's returned error into an IsError result and
					// keeps the error for middleware; argument validation failures take the
					// same path but are the caller's fault.
					outcome = OutcomeToolError
					if herr := r.GetError(); herr != nil {
						if !strings.HasPrefix(herr.Error(), `validating "arguments"`) {
							outcome = OutcomeError
						}
						attrs = append(attrs, "error", herr.Error())
					}
				}
				elapsed := time.Since(start)
				m.toolCalls.WithLabelValues(tool, outcome).Inc()
				m.toolDuration.WithLabelValues(tool).Observe(elapsed.Seconds())

				level := slog.LevelInfo
				if outcome == OutcomeError || outcome == OutcomePanic {
					level = slog.LevelError
				}
				attrs = append(attrs, "outcome", outcome, "duration_ms", elapsed.Milliseconds())
				logger.Log(ctx, level, "tool call", attrs...)
			}()
			return next(ctx, method, req)
		}
	}
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func requestIDFrom(req mcp.Request) string {
	if extra := req.GetExtra(); extra != nil && extra.Header != nil {
		if id := extra.Header.Get(RequestIDHeader); validRequestID.MatchString(id) {
			return id
		}
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// loggedArgs keeps valid JSON arguments as structured JSON in the log and
// truncates oversized or malformed ones to a string.
func loggedArgs(args json.RawMessage) any {
	if len(args) == 0 {
		return json.RawMessage("{}")
	}
	if len(args) <= maxLoggedArgs && json.Valid(args) {
		return args
	}
	s := string(args)
	if len(s) > maxLoggedArgs {
		s = s[:maxLoggedArgs] + "…"
	}
	return s
}

// UpstreamTransport wraps next with Navitia request metrics and logs failed
// requests (transport errors, 429 and 5xx) with the tool call's request id.
func UpstreamTransport(next http.RoundTripper, m *Metrics, logger *slog.Logger) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		endpoint := EndpointLabel(req.URL.Path)
		start := time.Now()
		resp, err := next.RoundTrip(req)
		elapsed := time.Since(start)

		code := "error"
		if err == nil {
			code = strconv.Itoa(resp.StatusCode)
		}
		m.upstreamRequests.WithLabelValues(endpoint, code).Inc()
		m.upstreamDuration.WithLabelValues(endpoint).Observe(elapsed.Seconds())
		if err != nil || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			attrs := []any{"request_id", RequestID(req.Context()), "endpoint", endpoint, "code", code, "duration_ms", elapsed.Milliseconds()}
			if err != nil {
				attrs = append(attrs, "error", err.Error())
			}
			logger.WarnContext(req.Context(), "upstream request failed", attrs...)
		}
		return resp, err
	})
}

// EndpointLabel maps a Navitia path to a low-cardinality endpoint name
// (stop area ids must not become label values).
func EndpointLabel(path string) string {
	for _, name := range []string{"places", "journeys", "departures", "disruptions"} {
		if strings.HasSuffix(path, "/"+name) {
			return name
		}
	}
	return "other"
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
