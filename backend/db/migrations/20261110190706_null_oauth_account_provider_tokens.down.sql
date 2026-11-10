-- The up migration discarded the stored provider tokens and their expiry, and
-- nothing kept a copy, so this cannot restore them. The columns were never
-- dropped, so there is no schema to put back; this is a deliberate no-op.
SELECT 1;
