-- calendar_tokens.token holds the plaintext feed token beside its hash, so the
-- signed-in owner can read the subscribe URLs again after the mint response
-- is gone. Lookup still goes through token_hash; this column is never
-- searched. A token is phcal_ plus 64 hex characters, 70 in all.
--
-- NULL for every row minted before this column existed: those tokens were
-- never stored and cannot be recovered, only replaced.
--
-- ADD COLUMN with no default is a catalog change, but it still takes an
-- ACCESS EXCLUSIVE lock on calendar_tokens. lock_timeout bounds the wait for
-- it, so a long-running transaction fails this migration instead of queueing
-- every later feed lookup behind it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE calendar_tokens ADD COLUMN token VARCHAR(70);
