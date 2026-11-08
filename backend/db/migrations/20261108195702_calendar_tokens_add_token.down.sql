SET LOCAL lock_timeout = '5s';

ALTER TABLE calendar_tokens DROP COLUMN IF EXISTS token;
