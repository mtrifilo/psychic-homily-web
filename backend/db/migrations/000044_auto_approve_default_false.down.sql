-- Revert auto_approve default back to true.
ALTER TABLE venue_source_configs ALTER COLUMN auto_approve SET DEFAULT true;
