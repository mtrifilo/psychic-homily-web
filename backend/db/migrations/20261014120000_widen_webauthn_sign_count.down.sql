-- Reverse 20261014120000_widen_webauthn_sign_count.up.sql. A counter above
-- the INTEGER range is stored as that range's maximum, and a sign-in whose
-- authenticator reports a counter above it fails its credential update again.
ALTER TABLE webauthn_credentials
    ALTER COLUMN sign_count TYPE INTEGER USING LEAST(sign_count, 2147483647);
