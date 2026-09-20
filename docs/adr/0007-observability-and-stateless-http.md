# ADR-0007: Operate the HTTP server: metrics, per-call logs, panic isolation, stateless transport

**Status:** accepted

## Context

Run as a hosted HTTP service, the server needs to be observable and debuggable
by whoever is on call. The on-call responder may be an agent. Four things were missing:

- no metrics, so there was nothing to alert on;
- logs said nothing about individual tool calls;
- a panic in a tool handler was not recovered: the Go SDK has no `recover`,
  so a single bad input could take the whole process down;
- the HTTP transport kept per-client sessions (ADR-0004), so an incoming
  call could not be replayed without the `initialize` handshake.

## Decision

- `internal/observability` wraps the server as MCP middleware and an HTTP round tripper:
  - Prometheus metrics: tool calls by `tool` and `outcome` (`ok`, `tool_error`, `error`,
    `panic`), latency histograms, upstream requests by endpoint and status, and
    `sncf_mcp_build_info{commit}` to tie behavior to a deploy;
  - one JSON log line per tool call: request id (from `X-Request-Id` or generated),
    tool, arguments, outcome, duration, error; panics also log their stack;
  - a recovered panic becomes a JSON-RPC error for that call only;
  - failed upstream requests (transport error, 429, 5xx) are logged with the request id.
- `outcome` separates the caller's mistakes (`tool_error`: unresolved station, invalid
  arguments) from ours (`error`, `panic`). Alerts are built on the latter only.
- The HTTP transport is stateless with plain JSON responses. Any logged call can be
  replayed with a single POST; stdio stays the default.
- `SNCF_API_BASE_URL` points the client at a recorded-fixture stand-in for hermetic runs.

## Consequences

- Server-initiated requests (sampling, elicitation) are unavailable over HTTP. This
  server does not use them.
- Tool arguments land in logs. Today they are station names; anything sensitive added
  later must be redacted before logging.
