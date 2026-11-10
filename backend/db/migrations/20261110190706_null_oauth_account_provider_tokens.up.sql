-- oauth_accounts holds no provider credential: the access token, the refresh
-- token and the token's expiry are NULL on every row. Nothing reads them, and
-- sign-in does not write them. The columns stay.
--
-- The UPDATE locks only the rows it changes, and a sign-in or link that holds
-- one of them waits it out. lock_timeout bounds that wait, so a long-running
-- transaction fails this migration instead of queueing later writes to the
-- same rows behind it.
SET LOCAL lock_timeout = '5s';

UPDATE oauth_accounts
SET access_token = NULL,
    refresh_token = NULL,
    expires_at = NULL
WHERE access_token IS NOT NULL
   OR refresh_token IS NOT NULL
   OR expires_at IS NOT NULL;
