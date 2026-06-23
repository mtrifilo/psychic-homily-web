-- PSY-1186: schedule_locked guards an admin-curated radio_shows.schedule from the weekly
-- WFMU schedule scrape (PSY-1159), which is otherwise scrape-wins. An admin edit to a
-- show's schedule (UpdateShow) sets schedule_locked=true; the scrape skips locked shows
-- and clear-on-absence leaves them alone. An admin clears the flag to resume auto-scraping.
--
-- ADDITIVE: BOOLEAN NOT NULL DEFAULT FALSE — Postgres adds a constant-default column
-- without a table rewrite (PG 11+). Existing rows default to unlocked (scrape-managed).
ALTER TABLE radio_shows
    ADD COLUMN schedule_locked BOOLEAN NOT NULL DEFAULT FALSE;
