-- calendar_link_mint_counts caps how many calendar links an account makes or
-- replaces in one clock hour. A row holds the account, the start of the
-- clock hour it last made a link in, and how many it made in that hour. It
-- holds no instant finer than the hour and nothing about any link, so it
-- cannot be lined up with a retired_calendar_links row. A later hour's first
-- mint overwrites the row, so nothing needs to sweep it, and the row is
-- deleted with its account.
--
-- REFERENCES users takes a lock on users that blocks writes to it;
-- lock_timeout bounds the wait for that lock, so a long-running transaction
-- fails this migration instead of queueing every write to users behind it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE calendar_link_mint_counts (
    user_id    INTEGER     PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mint_hour  TIMESTAMPTZ NOT NULL,
    mint_count INTEGER     NOT NULL,
    CONSTRAINT calendar_link_mint_counts_count_check CHECK (mint_count > 0)
);
