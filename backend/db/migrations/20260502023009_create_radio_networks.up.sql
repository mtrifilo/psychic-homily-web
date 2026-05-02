-- PSY-508: Create radio_networks table.
-- A network groups sibling radio_stations under a common brand
-- (e.g. WFMU 91.1 broadcast + Drummer / Sheena / Rock'n'Soul stream-only
-- sub-channels are all siblings under the WFMU network).
-- The network_id FK on radio_stations is added in a follow-up migration.

CREATE TABLE radio_networks (
    id BIGSERIAL PRIMARY KEY,
    slug VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
