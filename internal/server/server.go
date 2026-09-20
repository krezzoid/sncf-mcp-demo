// Package server wires the tool handlers into an MCP server and runs it over
// the selected transport. Transport choice is discussed in ADR-0004; the
// default is stdio, which is what local MCP clients (Claude Desktop, Cursor)
// launch directly.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/krezzoid/sncf-mcp/internal/navitia"
	"github.com/krezzoid/sncf-mcp/internal/observability"
	"github.com/krezzoid/sncf-mcp/internal/tools"
)

// Version is the server version reported in the MCP implementation info. It is
// "dev" for local builds and stamped to the release tag at build time via
// -ldflags "-X .../internal/server.Version=..." (see docs/RELEASING.md).
var Version = "dev"

// Commit is the git commit the binary was built from, stamped at build time via
// -ldflags "-X .../internal/server.Commit=...". Exported in sncf_mcp_build_info.
var Commit = "unknown"

// shutdownTimeout bounds how long in-flight HTTP requests may finish on shutdown.
const shutdownTimeout = 10 * time.Second

// Config holds runtime configuration for the server.
type Config struct {
	APIKey    string                 // SNCF API key
	Transport string                 // "stdio" (default) or "http"
	HTTPAddr  string                 // listen address when Transport == "http", e.g. ":8080"
	BaseURL   string                 // Navitia base URL; empty means navitia.DefaultBaseURL
	Logger    *slog.Logger           // nil means slog.Default()
	Metrics   *observability.Metrics // nil means a fresh set, served on /metrics in HTTP mode
}

func (cfg *Config) defaults() {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = observability.NewMetrics(Version, Commit)
	}
}

// build constructs the MCP server and registers the available tools.
//
// NOTE: the exact SDK constructor/transport names should be confirmed against
// the go-sdk version pinned in go.mod (run `go doc github.com/modelcontextprotocol/go-sdk/mcp`).
// The AddTool / handler signatures below follow the official v1 examples.
func build(cfg Config) *mcp.Server {
	opts := []navitia.Option{navitia.WithHTTPClient(&http.Client{
		Timeout:   15 * time.Second,
		Transport: observability.UpstreamTransport(http.DefaultTransport, cfg.Metrics, cfg.Logger),
	})}
	if cfg.BaseURL != "" {
		opts = append(opts, navitia.WithBaseURL(cfg.BaseURL))
	}
	client := navitia.New(cfg.APIKey, opts...)
	h := tools.New(client)

	srv := mcp.NewServer(&mcp.Implementation{Name: "sncf-mcp", Version: Version}, nil)
	srv.AddReceivingMiddleware(observability.ToolCalls(cfg.Logger, cfg.Metrics))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "find_station",
		Description: "Resolve a station or city name to ranked SNCF station candidates. Use it to disambiguate a name (e.g. 'Lyon') before plan_journey, then pass a returned id.",
	}, h.FindStation)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "plan_journey",
		Description: "Plan train itineraries between two locations on the French rail network. Accepts station/city names (resolved automatically) or stop_area ids from find_station.",
	}, h.PlanJourney)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "next_departures",
		Description: "List upcoming departures from a station on the French rail network, with real-time delays.",
	}, h.NextDepartures)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_disruptions",
		Description: "List active service disruptions on the French rail network, optionally scoped to a station.",
	}, h.GetDisruptions)

	return srv
}

// Run builds the server and serves it over the configured transport, blocking
// until the context is cancelled or the client disconnects.
func Run(ctx context.Context, cfg Config) error {
	cfg.defaults()
	srv := build(cfg)

	switch cfg.Transport {
	case "", "stdio":
		// Local clients launch the process and speak over stdin/stdout.
		return srv.Run(ctx, &mcp.StdioTransport{})
	case "http":
		return serveHTTP(ctx, cfg, srv)
	default:
		return fmt.Errorf("unknown transport %q (want 'stdio' or 'http')", cfg.Transport)
	}
}

// serveHTTP serves MCP over Streamable HTTP on every path except /metrics and
// /healthz. The handler is stateless with plain JSON responses (see ADR-0004):
// every tools/call is self-contained, so a load balancer, a load generator or a
// replay of a logged call needs no initialize handshake or session affinity.
func serveHTTP(ctx context.Context, cfg Config, srv *mcp.Server) error {
	// ReadHeaderTimeout guards against Slowloris-style attacks (gosec G114).
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpHandler(cfg, srv),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		// Let in-flight calls finish so a redeploy does not surface as errors.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func httpHandler(cfg Config, srv *mcp.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", cfg.Metrics.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/", mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: cfg.Logger},
	))
	return mux
}
