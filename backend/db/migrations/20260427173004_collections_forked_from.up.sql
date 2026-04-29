-- PSY-351: Collections v2 — clone/fork collections with attribution.
--
-- Add a self-referencing nullable FK from `collections` back to itself so a
-- forked collection can point at its source. ON DELETE SET NULL is the
-- explicit product decision: deleting the original must NOT cascade-delete
-- forks. The detail page renders a fallback ("Forked from a deleted
-- collection") when the FK is NULL.
--
-- Forks count is computed live (COUNT) on read, mirroring the existing
-- collection counter pattern (item_count, subscriber_count, contributor_count
-- are all live COUNTs — see CollectionService.batchCount* helpers).
-- A partial index on the new column keeps the COUNT cheap.

ALTER TABLE collections
    ADD COLUMN forked_from_collection_id BIGINT
    REFERENCES collections(id) ON DELETE SET NULL;

-- Partial index: only forked collections carry the column, so partial keeps
-- the index small and aligns with the queries that filter
-- `WHERE forked_from_collection_id = ?` for fork count.
CREATE INDEX idx_collections_forked_from
    ON collections(forked_from_collection_id)
    WHERE forked_from_collection_id IS NOT NULL;
