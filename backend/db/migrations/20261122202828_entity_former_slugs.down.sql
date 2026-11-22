-- Undoes 20261122202828 only: the trigger function and the table. The
-- triggers that run the function are dropped by the later migrations' downs,
-- which run first. Dropping the table takes an ACCESS EXCLUSIVE lock on it
-- alone; lock_timeout bounds the wait for it.
SET LOCAL lock_timeout = '5s';

DROP FUNCTION IF EXISTS entity_former_slugs_track();

DROP TABLE IF EXISTS entity_former_slugs;
