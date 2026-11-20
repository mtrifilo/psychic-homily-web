-- retired_calendar_links remembers a feed address after its token is deleted,
-- so a calendar app still polling it can be told why it stopped. A row holds
-- the SHA-256 hash calendar_tokens held for the token, why the link was
-- retired and when. It holds no account, no show and never the plaintext
-- token, and nothing accepts it as a credential: the feed handlers read it
-- only to answer an address calendar_tokens no longer knows.
--
-- A feed request finds its row through the token_hash unique index. A row
-- older than one year is treated as absent and is deleted the next time a
-- link is retired, through retired_calendar_links_retired_at_idx.
--
-- A new table takes no lock anyone else waits on; lock_timeout bounds the
-- index build all the same, as every migration's strong lock is bounded.
SET LOCAL lock_timeout = '5s';

CREATE TABLE retired_calendar_links (
    id         BIGSERIAL   PRIMARY KEY,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    reason     TEXT        NOT NULL,
    retired_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT retired_calendar_links_reason_check
        CHECK (reason IN ('reset', 'replaced', 'turned_off'))
);

CREATE INDEX retired_calendar_links_retired_at_idx ON retired_calendar_links (retired_at);
