-- Dropping this table makes shared.VenueTZJoin's zone guard raise "relation
-- does not exist" on every listing query, so rolling this migration back
-- requires rolling the application back with it.
DROP TABLE IF EXISTS timezone_names_snapshot;
