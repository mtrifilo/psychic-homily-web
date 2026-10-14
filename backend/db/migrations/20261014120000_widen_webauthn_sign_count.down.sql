-- Reverse 20261014120000_widen_webauthn_sign_count.up.sql. A counter above
-- the INTEGER range is stored as that range's maximum.
ALTER TABLE webauthn_credentials
    ALTER COLUMN sign_count TYPE INTEGER USING LEAST(sign_count, 2147483647);
