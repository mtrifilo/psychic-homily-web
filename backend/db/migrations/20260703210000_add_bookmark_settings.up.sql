-- PSY-1341: per-follow settings (first use: scene follows' notify mode —
-- 'all' vs 'followed_bands_only'). JSONB so future follow-scoped preferences
-- don't each need a column; NULL means "all defaults".
ALTER TABLE user_bookmarks ADD COLUMN settings JSONB;
