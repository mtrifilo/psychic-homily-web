-- PSY-1682: persist a venue's house-default age policy.
--
-- shows.age_requirement already carries the PER-EVENT rule ("21+" for one
-- booking at an otherwise all-ages room). What was missing is the room's own
-- standing policy, which is what a reader needs when a show carries no
-- override, and what makes the override legible as an override.
--
-- Free text, mirroring shows.age_requirement, because the real-world vocabulary
-- is open ("all ages", "17+", "21+", "18+ w/ guardian") and an enum would force
-- lossy coercion at ingest time on data we collect from humans. If a controlled
-- vocabulary is wanted later it can be layered on top of the observed values.
--
-- Nullable with no default and no backfill: unknown for every existing row.
-- Population happens through the community edit flow (the venue suggest-edit
-- allowlist), the same way capacity is curated. Adding a nullable column with
-- no default is metadata-only in Postgres 11+, so no table rewrite.
ALTER TABLE venues
    ADD COLUMN age_policy TEXT;
