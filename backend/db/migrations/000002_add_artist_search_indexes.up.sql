-- Enable pg_trgm extension for trigram-based autocomplete search
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- GIN index for fuzzy matching (anywhere in the string)
-- This enables ILIKE '%term%' queries to be blazing fast
CREATE INDEX idx_artists_name_trgm ON artists USING gin (name gin_trgm_ops);

-- B-tree index for case-sensitive prefix searches
CREATE INDEX idx_artists_name_prefix ON artists (name text_pattern_ops);

-- Functional B-tree index for case-INSENSITIVE prefix searches
-- This enables fast LOWER(name) LIKE 'term%' queries
CREATE INDEX idx_artists_name_lower_prefix ON artists (LOWER(name) text_pattern_ops);

-- Comments for documentation
COMMENT ON INDEX idx_artists_name_trgm IS 'Trigram GIN index for fast fuzzy autocomplete search on artist names';
COMMENT ON INDEX idx_artists_name_prefix IS 'B-tree index for case-sensitive prefix searches (LIKE ''term%'')';
COMMENT ON INDEX idx_artists_name_lower_prefix IS 'B-tree index for case-insensitive prefix searches (LOWER(name) LIKE ''term%'')';
