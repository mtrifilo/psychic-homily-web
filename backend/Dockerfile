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
# stage exists so only a resolved node_modules tree crosses into the runtime, and
# no npm cache does. The runtime base ships npm itself, which the final stage
# strips.
#
# `npm ci` (not `npm install`) so the image is built from the committed
# package-lock.json — the layout's determinism contract starts with the exact
# graphology-layout-forceatlas2 build the stability parameters were measured
# against. Only package.json and the lockfile are copied in, so a source change
# in the Go tree does not invalidate this layer.
#
# Pinned to the same base as the runtime stage (see NODE PIN below), so the npm
# that resolves the tree and the node that executes the script are one build.
FROM node:22.23.2-alpine3.24@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS layout-deps
WORKDIR /layout
COPY layout/package.json layout/package-lock.json ./
RUN npm ci --omit=dev
COPY layout/fa2-layout.mjs ./

# Final stage
#
# NODE PIN. ForceAtlas2 runs in Node (see the layout-deps stage above), and a V8
# upgrade is allowed to change float results at the margins. Note where the
# exposure actually is: an unchanged graph never reaches the physics at all,
# because graph_overview.go short-circuits on structure_key and replays the
# stored positions verbatim. The risk is the nights that DO run a layout. Their
# output becomes the warm-start baseline every later epoch resumes from, so a V8
# float change there silently rebases the whole map once, permanently. Pinning
# makes that a deliberate event rather than a side effect of whenever the image
# last rebuilt, and it lets the determinism assertions in CI's `node --test` run
# against the same V8 production uses.
#
# Mechanism: an official node-alpine image at its full patch tag, plus the
# digest of the tag's manifest list. The tag documents the version for a human;
# the digest is what actually makes the pull immutable, because even a full
# patch tag is republished when its Alpine base is patched.
#
# The digest MUST be the manifest-list (OCI image index) digest, not a
# per-platform one: Railway builds linux/amd64 while local dev is often
# linux/arm64, and a platform digest would fail the pull on the other arch.
# `docker buildx imagetools inspect` prints the index digest at the top.
#
# Cost, and it is not small. Measured by summing uncompressed layers for
# linux/amd64: ~197MB before, ~291MB after, so +94MB (+48%). Nearly all of it is
# the official node build's 160MB layer, which ships a full-ICU node binary where
# Alpine's apk `nodejs` links shared libraries instead.
#
# Rejected the cheaper option, pinning `alpine:3.XX` by digest and keeping
# `apk add nodejs`: measured at ~197MB, so it saves the whole 94MB, but the apk
# index moves within a release branch, so `nodejs` still floats across Node
# patches, and pinning `nodejs=<exact>` rots as soon as that version leaves the
# index. It buys back the size by pinning nothing, which is the one thing this
# exists to do. Also rejected: hand-copying the node binary onto a plain Alpine
# base, which measured only ~5MB better than this and moves 50MB out of a
# registry-shared official blob into an app-specific layer.
#
# Scope, so this is not mistaken for completeness. Still floating, deliberately:
# the builder's `golang:1.24-alpine`, and Go's math.Sin/Cos/Hypot sit in the
# seeding and Procrustes paths, so the Go side is a second float input. It is
# accepted as lower risk, and pinning it would have to move six CI `go-version`
# lines in lockstep. The apk packages below also stay unpinned so security fixes
# keep arriving. Freezing this base does mean Alpine's own patches now wait for a
# manual bump, which is the trade reproducibility asks for.
#
# THIS FILE IS THE SOURCE OF TRUTH for the Node version. CI does not restate
# it: the graph-layout step in .github/workflows/ci.yml derives it from the FROM
# lines below via `scripts/check_node_base_pin.sh --print-version`, so there is
# no second value that a bump can leave behind.
#
# Bumps are proposed weekly by Dependabot (.github/dependabot.yml), which
# watches these FROM lines, so a republished tag (an Alpine patch) or a new Node
# patch arrives as a PR instead of never arriving. Three checks stand behind it,
# all runnable locally, all described in their own headers:
#   scripts/check_node_base_pin.sh       shape of the pin (Backend Tests, required)
#   scripts/check_node_base_manifest.sh  digest is a multi-arch manifest list
#   .github/workflows/backend-image.yml  the image still builds, before Railway
#
# To bump (deliberately, not incidentally):
#   1. docker buildx imagetools inspect node:<new-version>-alpine<alpine-ver>
#   2. copy the TOP-LEVEL `Digest:` into BOTH FROM lines in this file
#   3. scripts/check_node_base_pin.sh && scripts/check_node_base_manifest.sh
#   4. rebuild and confirm `node --test` in backend/layout still passes
FROM node:22.23.2-alpine3.24@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32

# ca-certificates for HTTPS requests, tzdata for /usr/share/zoneinfo (absent
# from this base), netcat-openbsd for the entrypoint's database wait. Node comes
# from the base image above.
#
# The same layer strips the package managers the base ships, so the runtime keeps
# the property the layout-deps stage was built for: the worker needs the node
# interpreter only, and its dependencies arrive as an already-resolved tree. This
# is attack-surface reduction and NOT a size win. The bytes live in the immutable
# base layer, so removing them here only changes the filesystem view. The
# /usr/local/bin entries are symlinks into the trees above, removed so nothing on
# PATH is left dangling.
RUN apk --no-cache add ca-certificates tzdata netcat-openbsd && \
    rm -rf /usr/local/lib/node_modules /opt/yarn-v* \
           /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack \
           /usr/local/bin/yarn /usr/local/bin/yarnpkg

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
