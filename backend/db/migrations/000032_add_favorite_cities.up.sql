ALTER TABLE user_preferences
ADD COLUMN favorite_cities JSONB NOT NULL DEFAULT '[]'::jsonb;
