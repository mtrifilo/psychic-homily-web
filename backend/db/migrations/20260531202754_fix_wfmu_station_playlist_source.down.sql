-- PSY-927: No-op (intentional).
--
-- This is a one-way data correction. 'wfmu_html' was an invalid value that
-- silently broke playlist import — never a state worth restoring. The up
-- migration only ever rewrites the broken value and is a no-op on environments
-- that were never affected (including the fresh CI database), so there is
-- nothing to reverse. A precise reverse is impossible anyway: once corrected,
-- the row is indistinguishable from the legitimately-'wfmu_scrape' sub-stations.
-- Down intentionally does nothing.
SELECT 1;
