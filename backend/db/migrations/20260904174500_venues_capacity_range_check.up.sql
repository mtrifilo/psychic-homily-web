-- PSY-1706: venues.capacity carries its range at the column.

-- The three write paths (admin create, admin update, contributor suggest-edit)
-- already range-check against contracts.MinVenueCapacity / MaxVenueCapacity
-- (backend/internal/services/contracts/catalog.go), so this constraint refuses
-- nothing those paths would send. It exists for the path that does not: an
-- ingest CLI, a hand-run UPDATE, or a future writer that skips the shared
-- bounds stores junk silently without it.
--
-- The numbers mirror contracts.MinVenueCapacity (1) and
-- contracts.MaxVenueCapacity (200000) and change together with them.
-- The floor is 1 rather than 0 because NULL already means "capacity unknown",
-- so a stored 0 would be a second spelling of the same fact, and one that reads
-- downstream as a known one.
--
-- NULL stays legal: an unknown capacity is the common case, not a violation.
--
-- Adding the constraint takes a brief ACCESS EXCLUSIVE lock on venues and
-- validates existing rows.

ALTER TABLE venues
    ADD CONSTRAINT venues_capacity_range
    CHECK (capacity IS NULL OR capacity BETWEEN 1 AND 200000);

COMMENT ON COLUMN venues.capacity IS
    'Room capacity in people, or NULL when unknown. Constrained to 1..200000, mirroring contracts.MinVenueCapacity / MaxVenueCapacity.';
