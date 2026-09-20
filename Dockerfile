# Multi-stage build: compile a static binary, then ship it on a distroless base
# for a minimal attack surface (no shell, no package manager). See the security
# section of the README.

# --- build stage ---------------------------------------------------------
FROM golang:1.26 AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
# CGO disabled => fully static binary that runs on a scratch/distroless image.
# VERSION and COMMIT are stamped into internal/server (defaults "dev"/"unknown");
# COMMIT shows up in sncf_mcp_build_info, which ties metrics to a deploy.
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
	-ldflags="-s -w -X github.com/krezzoid/sncf-mcp/internal/server.Version=${VERSION} -X github.com/krezzoid/sncf-mcp/internal/server.Commit=${COMMIT}" \
	-o /out/sncf-mcp ./cmd/sncf-mcp

# --- runtime stage -------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sncf-mcp /usr/local/bin/sncf-mcp
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/sncf-mcp"]
