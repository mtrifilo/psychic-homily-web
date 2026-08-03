# Build stage
FROM golang:1.24-alpine AS builder

# Install git and ca-certificates (needed for go mod download)
RUN apk add --no-cache git ca-certificates

# Download golang-migrate CLI
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -a -installsuffix cgo \
    -o main ./cmd/server

# Layout worker dependencies.
#
# The nightly graph-overview snapshot runs ForceAtlas2 in a Node subprocess
# (internal/services/catalog/graph_overview_layout.go → layout/fa2-layout.mjs)
# because there is no maintained pure-Go implementation with Barnes-Hut. This
# stage exists so the runtime image never carries npm or a package cache: only
# the resolved node_modules tree crosses into it.
#
# `npm ci` (not `npm install`) so the image is built from the committed
# package-lock.json — the layout's determinism contract starts with the exact
# graphology-layout-forceatlas2 build the stability parameters were measured
# against. Only package.json and the lockfile are copied in, so a source change
# in the Go tree does not invalidate this layer.
FROM node:22-alpine AS layout-deps
WORKDIR /layout
COPY layout/package.json layout/package-lock.json ./
RUN npm ci --omit=dev
COPY layout/fa2-layout.mjs ./

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests, netcat for health checks, and
# nodejs for the layout worker above (interpreter only — npm stays in the
# builder stage).
RUN apk --no-cache add ca-certificates tzdata netcat-openbsd nodejs

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/main .

# Copy migrate CLI from builder stage
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate

# Copy migrations
COPY --from=builder /app/db/migrations ./db/migrations

# Copy the layout worker and its resolved dependencies. The Go job resolves the
# script at ./layout/fa2-layout.mjs relative to WORKDIR by default, so this path
# is the contract (GRAPH_LAYOUT_SCRIPT overrides it).
COPY --from=layout-deps /layout ./layout

# Copy entrypoint script
COPY docker-entrypoint.sh .

# Change ownership to non-root user and make entrypoint executable
RUN chown -R appuser:appgroup /app && \
    chmod +x /app/docker-entrypoint.sh

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the entrypoint script
ENTRYPOINT ["/app/docker-entrypoint.sh"]
