-- Nightly precomputed "Map of the Scene" layout snapshot.
--
-- APPEND-ONLY, READ-NEWEST. The nightly job INSERTs a new row and prunes older
-- ones; it never mutates a live row. That gives the same guarantee
-- artist_communities gets from its transactional swap — a reader is never shown
-- a half-written map — with a stronger failure posture: a job that dies
-- anywhere before COMMIT leaves the previous snapshot untouched and still the
-- newest row, so there is no window in which the map is missing.
--
-- Readers fetch in ONE statement (ORDER BY id DESC LIMIT 1), so they cannot
-- straddle the insert.
CREATE TABLE graph_overview_snapshots (
    id BIGSERIAL PRIMARY KEY,

    -- payload is the complete serving body for GET /graph/overview: quantized
    -- node positions, community regions and hulls, CSR edge arrays, appear
    -- times. Stored whole so serving is a single row read with no assembly.
    payload JSONB NOT NULL,

    -- layout is the warm-start input for the NEXT epoch: full float32-precision
    -- positions keyed by node id. Kept out of payload because it is producer
    -- state, not client data — the client gets the quantized copy, and mixing
    -- the two would ship every coordinate twice.
    layout JSONB NOT NULL,

    node_count INTEGER NOT NULL,
    edge_count INTEGER NOT NULL,
    isolate_count INTEGER NOT NULL,

    -- content_hash identifies the snapshot and is served as the ETag. It is a
    -- digest of the payload as marshalled at build time, which is NOT byte-equal
    -- to what a reader gets back (this column is JSONB, so Postgres normalizes
    -- key order and whitespace) nor to what is served (the API adds a $schema
    -- member). It is an identity stamp for the build, which is all a validator
    -- needs: the map only ever changes by a new row appearing.
    content_hash TEXT NOT NULL,

    -- structure_key digests the node set and edge set the layout was computed
    -- from, and nothing else. When the next build's key matches, the graph's
    -- SHAPE is unchanged and the previous positions are reused verbatim rather
    -- than relaxed for another 50 ForceAtlas2 iterations — which is what makes
    -- an unchanged night reproduce a byte-identical map instead of one that
    -- drifts slightly every time the job runs.
    structure_key TEXT NOT NULL DEFAULT '',

    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The only read pattern is "newest snapshot", which the PK index already serves
-- via ORDER BY id DESC LIMIT 1 — as does the retention prune. This index exists
-- for operators asking when the map was last built.
CREATE INDEX idx_graph_overview_snapshots_computed_at
    ON graph_overview_snapshots (computed_at DESC);
