-- Add extraction_notes to venue_source_configs for per-venue AI extraction hints.
-- Admin-editable notes are appended to the AI prompt to improve extraction quality
-- (e.g., "skip karaoke Tuesdays" or "this venue hosts trivia every Wednesday").
ALTER TABLE venue_source_configs
    ADD COLUMN extraction_notes TEXT;
