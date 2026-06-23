-- PSY-855: Postgres-backed per-user rate limit for the AI extraction routes
-- (extract-show + extract-collection). The Next.js BFF routes call the Go
-- backend before doing any Anthropic work; the backend atomically increments
-- and checks this rolling-window counter so the limit is consistent across
-- Vercel serverless instances (an in-memory counter would not be).
--
-- One row per user — a fixed 1-hour rolling window (window_start..window_start+1h)
-- carrying request_count. The throttle service resets the window in-place when
-- it has elapsed (CASE in the upsert) rather than inserting a new row, so the
-- table never grows beyond one row per user who has ever extracted.
--
-- ADDITIVE: one brand-new table; nothing existing is touched. Multi-statement
-- file => golang-migrate wraps it in a transaction => no CREATE INDEX
-- CONCURRENTLY (illegal in a txn, and unnecessary on an empty new table).

CREATE TABLE ai_extraction_throttle (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    -- Start of the current rolling window. request_count accrues against this
    -- instant; once NOW() >= window_start + interval '1 hour' the service resets
    -- window_start to NOW() and request_count to 1 in a single atomic upsert.
    window_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
