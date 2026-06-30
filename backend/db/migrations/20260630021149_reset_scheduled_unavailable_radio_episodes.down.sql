-- PSY-1285: No-op (intentional).
--
-- This is a one-way data correction: 'unavailable'/burned-attempts on a not-yet-aired
-- episode was an invalid state (a windowless give-up wrongly applied to a row later given
-- a future window), never one worth restoring, and a corrected row is indistinguishable
-- from a freshly-imported pending one. Down intentionally does nothing.
SELECT 1;
