-- oauth_accounts keeps no provider credential: this sets the access token, the
-- refresh token and the token's expiry to NULL on every row that holds any of
-- them. Nothing reads them, and no path copies them from a provider. The
-- columns stay.
--
-- The UPDATE locks only the rows it changes, and waits for a link, attach or
-- unlink that holds one of them. lock_timeout bounds that wait, so a
-- long-running transaction fails this migration instead of queueing later
-- writes to the same rows behind it.
SET LOCAL lock_timeout = '5s';

UPDATE oauth_accounts
SET access_token = NULL,
    refresh_token = NULL,
    expires_at = NULL
WHERE access_token IS NOT NULL
   OR refresh_token IS NOT NULL
   OR expires_at IS NOT NULL;
