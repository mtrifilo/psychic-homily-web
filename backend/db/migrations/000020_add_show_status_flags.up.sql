-- Add status flags for sold out and cancelled shows
ALTER TABLE shows
ADD COLUMN is_sold_out BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN is_cancelled BOOLEAN NOT NULL DEFAULT FALSE;
