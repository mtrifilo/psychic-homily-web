-- PSY-1287: No-op (intentional).
--
-- This is a one-way data correction: a windowless episode stranded 'unavailable'/burned-attempts
-- BEFORE it aired was an invalid state (the give-up ran before the broadcast), never one worth
-- restoring, and a corrected row is indistinguishable from a freshly-imported pending one. Down
-- intentionally does nothing — mirrors the PSY-1285 correction migration's no-op down.
SELECT 1;
