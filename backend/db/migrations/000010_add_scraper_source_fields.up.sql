-- Add source tracking fields for scraped shows
-- Allows tracking where shows came from (user submission vs scraper)
-- and enables deduplication for scraped events

-- Create enum type for show source
CREATE TYPE show_source AS ENUM ('user', 'scraper');

-- Add source column to shows table (default 'user' for existing and manually submitted shows)
ALTER TABLE shows ADD COLUMN source show_source NOT NULL DEFAULT 'user';

-- Add source_venue to identify which venue scraper the event came from (e.g., 'valley-bar', 'crescent-ballroom')
ALTER TABLE shows ADD COLUMN source_venue VARCHAR(100);

-- Add source_event_id for the external event ID from the scraped source (for deduplication)
ALTER TABLE shows ADD COLUMN source_event_id VARCHAR(255);

-- Add scraped_at timestamp to track when the event was scraped
ALTER TABLE shows ADD COLUMN scraped_at TIMESTAMP WITH TIME ZONE;

-- Create unique constraint for deduplication: same venue source + event ID should not be imported twice
-- This is a partial unique index that only applies when source_venue and source_event_id are not null
CREATE UNIQUE INDEX idx_shows_source_dedup ON shows(source_venue, source_event_id)
WHERE source_venue IS NOT NULL AND source_event_id IS NOT NULL;

-- Create index for filtering by source
CREATE INDEX idx_shows_source ON shows(source);

-- Add comments for documentation
COMMENT ON COLUMN shows.source IS 'Source of the show: user (manually submitted) or scraper (automated import)';
COMMENT ON COLUMN shows.source_venue IS 'Venue scraper identifier (e.g., valley-bar, crescent-ballroom) for scraped shows';
COMMENT ON COLUMN shows.source_event_id IS 'External event ID from the scraped source for deduplication';
COMMENT ON COLUMN shows.scraped_at IS 'Timestamp when the event was scraped from the source';
