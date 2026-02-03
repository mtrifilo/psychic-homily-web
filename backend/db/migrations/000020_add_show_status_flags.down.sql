-- Remove status flags for sold out and cancelled shows
ALTER TABLE shows
DROP COLUMN is_sold_out,
DROP COLUMN is_cancelled;
