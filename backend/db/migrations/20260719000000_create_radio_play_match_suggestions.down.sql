-- PSY-1494: reverse radio_play_match_suggestions. DROP TABLE removes indexes,
-- CHECKs, and FKs with it.

DROP TABLE IF EXISTS radio_play_match_suggestions;
