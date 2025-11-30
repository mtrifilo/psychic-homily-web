-- Drop all tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS user_preferences CASCADE;
DROP TABLE IF EXISTS oauth_accounts CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS show_artists CASCADE;
DROP TABLE IF EXISTS shows CASCADE;
DROP TABLE IF EXISTS venues CASCADE;
DROP TABLE IF EXISTS artists CASCADE;
