CREATE INDEX idx_venues_name_trgm ON venues USING gin (name gin_trgm_ops);
CREATE INDEX idx_venues_name_prefix ON venues (name text_pattern_ops);
CREATE INDEX idx_venues_name_lower_prefix ON venues (LOWER(name) text_pattern_ops);

-- Comments for documentation
COMMENT ON INDEX idx_venues_name_trgm IS 'Trigram GIN index for fast fuzzy autocomplete search on venue names';
COMMENT ON INDEX idx_venues_name_prefix IS 'B-tree index for case-sensitive prefix searches (LIKE ''term%'')';
COMMENT ON INDEX idx_venues_name_lower_prefix IS 'B-tree index for case-insensitive prefix searches (LOWER(name) LIKE ''term%'')';
