-- Retire the hardcoded 'opener' default on show_artists.set_type, and leave the
-- column holding only values the PSY-1673 vocabulary defines.
--
-- Until PSY-1673 the show create/edit path stamped set_type = 'opener' on EVERY
-- non-headliner, and no surface in the product ever let a human choose a
-- different value. Below the headliner line the column described the code's
-- default rather than the bill: an "(opener)" annotation would have fired on
-- essentially every support act on the site.
--
-- Stage census recorded on the ticket, before this migration:
--
--   headliner  1513
--   opener      975
--   (no other value present)
--
-- WHY 'opener' IS NOT SALVAGEABLE, precisely.
--
-- It is tempting to say every 'opener' row is the hardcoded default. That is
-- not quite true, and the honest version matters. There were TWO writers:
--
--   1. the create/edit default, which meant nothing at all, and
--   2. the discovery importer, whose old normalizeSetType collapsed BOTH an
--      AI-extracted "support" AND an AI-extracted "opener" onto 'opener'.
--
-- So a stored 'opener' is ambiguous three ways -- "nobody said", "the source
-- said support", "the source said opener" -- and nothing in the row
-- distinguishes them. The value cannot be recovered, only guessed at.
--
-- Keeping it would therefore assert "this act opened" for rows where no one
-- ever said so, which is exactly the defect this ticket removes. Folding it
-- into 'performer' ("on the bill, slot unknown") asserts only what is actually
-- known. Discovery-sourced shows can recover real roles later by re-running
-- extraction, which now maps "support" to 'direct_support' instead of
-- flattening it. This trade was made explicitly on the ticket.
--
-- Headliner rows are the one inference the product still sanctions (position 0
-- / is_headliner) and are deliberately untouched, so no show loses its
-- headliner here.
--
-- 'opener' remains a legal, meaningful value going forward -- it is in the
-- vocabulary the API accepts. What changes is that from here on it is only
-- ever written when somebody explicitly says an act opened.

UPDATE show_artists
SET set_type = 'performer'
WHERE set_type = 'opener';

-- "support" was written by older dev seeds and by any environment whose data
-- predates the vocabulary. It states a real role, so it is mapped rather than
-- discarded -- the same mapping contracts.NormalizeSetType applies at ingest.
UPDATE show_artists
SET set_type = 'direct_support'
WHERE set_type = 'support';

-- Everything else lands on the neutral default so the column provably holds
-- only vocabulary values afterwards. This covers NULLs (the column is nullable
-- and the Go model scans it as a non-pointer string) and stale labels the
-- vocabulary does not model, such as the 'host' the old seed exemplars wrote.
--
-- Without this, an unrecognized value would survive as a trap: the show form
-- coerces what it cannot render to 'performer' for display and sends that back
-- on save, so editing such a show would silently overwrite the stored role.
UPDATE show_artists
SET set_type = 'performer'
WHERE set_type IS NULL
   OR set_type NOT IN (
        'headliner',
        'direct_support',
        'opener',
        'special_guest',
        'dj',
        'performer'
      );
