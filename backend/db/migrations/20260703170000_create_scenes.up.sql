-- PSY-1339: lazily-materialized scenes registry (the PSY-1314 spike decision).
-- Scenes stay computed aggregations; a row exists here only once something
-- id-keyed references the scene (first follow, a curated description, future
-- comments/tags). Identity anchor is the scope: the CBSA metro code for US
-- metro scenes, the literal (city, state) for fallback scenes. The slug is a
-- DISPLAY artifact (principal city) — unique for lookups, but not the anchor.
CREATE TABLE scenes (
    id BIGSERIAL PRIMARY KEY,
    metro VARCHAR(10),
    city VARCHAR(255) NOT NULL,
    state VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per scope. Partial unique indexes instead of a NULLS NOT DISTINCT
-- constraint so the schema doesn't require Postgres 15.
CREATE UNIQUE INDEX idx_scenes_metro ON scenes (metro) WHERE metro IS NOT NULL;
CREATE UNIQUE INDEX idx_scenes_city_state ON scenes (city, state) WHERE metro IS NULL;
CREATE UNIQUE INDEX idx_scenes_slug ON scenes (slug);
