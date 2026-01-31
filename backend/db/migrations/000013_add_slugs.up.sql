-- Add slug columns to artists, venues, and shows tables for SEO-friendly URLs

-- Artists: slug based on name (e.g., "the-national")
ALTER TABLE artists ADD COLUMN slug VARCHAR(255);

-- Venues: slug based on name + city + state (e.g., "valley-bar-phoenix-az")
ALTER TABLE venues ADD COLUMN slug VARCHAR(255);

-- Shows: slug based on date + headliner + venue (e.g., "2026-01-30-the-national-at-valley-bar")
ALTER TABLE shows ADD COLUMN slug VARCHAR(255);

-- Create unique indexes (partial index allowing NULL for migration flexibility)
CREATE UNIQUE INDEX idx_artists_slug ON artists(slug) WHERE slug IS NOT NULL;
CREATE UNIQUE INDEX idx_venues_slug ON venues(slug) WHERE slug IS NOT NULL;
CREATE UNIQUE INDEX idx_shows_slug ON shows(slug) WHERE slug IS NOT NULL;
