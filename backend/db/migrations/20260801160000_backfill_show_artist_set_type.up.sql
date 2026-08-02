-- Retire the hardcoded 'opener' default on show_artists.set_type.
--
-- Until PSY-1673 the show create/edit path and the discovery importer both
-- stamped set_type = 'opener' on EVERY non-headliner, and no surface anywhere
-- in the product ever let a human choose a different value. The column
-- therefore carried no information below the headliner line: an "(opener)"
-- annotation would have fired on essentially every support act on the site,
-- describing the code's default rather than the bill.
--
-- Stage census taken 2026-08-01, before this migration:
--
--   headliner  1513
--   opener      975
--   (no other value present, zero NULLs)
--
-- Because the only writer of 'opener' was that hardcoded default, all 975 rows
-- are provably uncurated. Moving them to 'performer' -- which the schema
-- already declares as the column DEFAULT and which means "on the bill, slot
-- unknown" -- is lossless: it discards a value that never encoded a human
-- judgment, and it stops the display layer from inheriting a guess.
--
-- Scoped exactly to 'opener'. Headliner rows are the one inference the product
-- still sanctions (position 0 / is_headliner) and are left untouched, as is
-- any row already holding a curated value.
--
-- 'opener' remains a legal, meaningful value going forward -- it is in the
-- vocabulary the API now accepts. What changes is that from here on it is
-- only ever written when somebody explicitly says an act opened.

UPDATE show_artists
SET set_type = 'performer'
WHERE set_type = 'opener';
