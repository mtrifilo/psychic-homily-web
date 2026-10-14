-- An authenticator's signature counter is an unsigned 32-bit value. BIGINT
-- holds all of it; INTEGER stops at 2^31 - 1.
ALTER TABLE webauthn_credentials ALTER COLUMN sign_count TYPE BIGINT;
