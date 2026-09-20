// Command sncf-mcp is a Model Context Protocol server exposing the French
// railway (SNCF) journey-planning API to AI agents.
//
// Usage:
//
//	SNCF_API_KEY=xxxx sncf-mcp                 # stdio (default)
//	SNCF_API_KEY=xxxx sncf-mcp -transport http -addr :8080 -log-format json
//
// SNCF_API_BASE_URL overrides the Navitia endpoint (e.g. a recorded-fixture stand-in).
package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	// Embed the timezone database so station times render with the correct
	// Europe/Paris offset even on a minimal image (distroless) that ships no
	// zoneinfo. See ADR-0005.
	_ "time/tzdata"

	"github.com/krezzoid/sncf-mcp/internal/server"
)

func main() {
	var (
		transport = flag.String("transport", "stdio", "transport: stdio | http")
		addr      = flag.String("addr", ":8080", "listen address when -transport=http")
		logFormat = flag.String("log-format", "text", "log format: text | json")
		logFile   = flag.String("log-file", "", "also append logs to this file")
	)
	flag.Parse()

	// Structured logging to stderr. stdout is reserved for the stdio transport,
	// so nothing else may write there.
	var out io.Writer = os.Stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			slog.Error("cannot open log file", "path", *logFile, "err", err)
			os.Exit(1)
		}
		defer func() { _ = f.Close() }()
		out = io.MultiWriter(os.Stderr, f)
	}
	var handler slog.Handler = slog.NewTextHandler(out, nil)
	if *logFormat == "json" {
		handler = slog.NewJSONHandler(out, nil)
	}
	logger := slog.New(handler).With("service", "sncf-mcp", "commit", server.Commit)
	slog.SetDefault(logger)

	apiKey := os.Getenv("SNCF_API_KEY")
	if apiKey == "" {
		slog.Error("SNCF_API_KEY is not set; get a free key at https://numerique.sncf.com/startup/api/")
		os.Exit(1)
	}

	// Cancel the context on SIGINT/SIGTERM for a clean shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := server.Config{
		APIKey:    apiKey,
		Transport: *transport,
		HTTPAddr:  *addr,
		BaseURL:   os.Getenv("SNCF_API_BASE_URL"),
		Logger:    logger,
	}

	slog.Info("starting sncf-mcp", "transport", *transport, "version", server.Version)
	if err := server.Run(ctx, cfg); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("sncf-mcp stopped")
}
