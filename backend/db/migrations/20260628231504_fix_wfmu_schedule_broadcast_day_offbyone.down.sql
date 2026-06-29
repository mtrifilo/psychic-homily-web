-- PSY-1283: No-op (intentional).
--
-- This is a one-way data correction. A precise reverse — shifting the same pre-6am slots
-- back by one day — would RE-INTRODUCE the broadcast-day off-by-one, a state never worth
-- restoring. It is also unsafe: once the corrected schedule has been re-written by a normal
-- scrape, a corrected row is indistinguishable from a freshly-scraped-correct one, so a blind
-- -1 down would corrupt legitimately-correct data. Down intentionally does nothing.
SELECT 1;
