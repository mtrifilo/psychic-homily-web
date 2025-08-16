-- Create artists table
CREATE TABLE artists (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    state VARCHAR(10),
    city VARCHAR(255),
    instagram VARCHAR(255),
    facebook VARCHAR(255),
    twitter VARCHAR(255),
    youtube VARCHAR(255),
    spotify VARCHAR(255),
    soundcloud VARCHAR(255),
    bandcamp VARCHAR(255),
    website VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create venues table
CREATE TABLE venues (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    address VARCHAR(500),
    city VARCHAR(255),
    state VARCHAR(10),
    zipcode VARCHAR(20),
    instagram VARCHAR(255),
    facebook VARCHAR(255),
    twitter VARCHAR(255),
    youtube VARCHAR(255),
    spotify VARCHAR(255),
    soundcloud VARCHAR(255),
    bandcamp VARCHAR(255),
    website VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create shows table
CREATE TABLE shows (
    id SERIAL PRIMARY KEY,
    title VARCHAR(500),
    event_date TIMESTAMP NOT NULL,
    city VARCHAR(255),
    state VARCHAR(10),
    price DECIMAL(10, 2),
    age_requirement VARCHAR(255),
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Junction table for many-to-many relationship: Shows ↔ Venues
CREATE TABLE show_venues (
    show_id INT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    PRIMARY KEY (show_id, venue_id)
);

-- Junction table for many-to-many relationship: Shows ↔ Artists (with ordering)
CREATE TABLE show_artists (
    show_id INT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    artist_id INT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    position INT NOT NULL DEFAULT 0, -- 0 = headliner, 1+ = openers in order
    set_type VARCHAR(50) DEFAULT 'performer', -- 'headliner', 'opener', 'special guest'
    PRIMARY KEY (show_id, artist_id)
);

-- Create users table for authentication
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    username VARCHAR(100) UNIQUE,
    password_hash VARCHAR(255), -- For local authentication
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    avatar_url VARCHAR(500),
    bio TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create OAuth accounts table (Goth compatible)
CREATE TABLE oauth_accounts (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL, -- 'google', 'github', etc.
    provider_user_id VARCHAR(255) NOT NULL, -- External provider's user ID
    provider_email VARCHAR(255),
    provider_name VARCHAR(255),
    provider_avatar_url VARCHAR(500),
    access_token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_user_id) -- One account per provider per external user
);

-- Create user preferences table
CREATE TABLE user_preferences (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_email BOOLEAN NOT NULL DEFAULT TRUE,
    notification_push BOOLEAN NOT NULL DEFAULT FALSE,
    theme VARCHAR(50) NOT NULL DEFAULT 'light',
    timezone VARCHAR(50) NOT NULL DEFAULT 'UTC',
    language VARCHAR(10) NOT NULL DEFAULT 'en',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);

-- Indexes for performance
CREATE INDEX idx_artists_name ON artists(name);
CREATE INDEX idx_venues_name ON venues(name);
CREATE INDEX idx_shows_event_date ON shows(event_date);
CREATE INDEX idx_shows_city ON shows(city);
CREATE INDEX idx_show_venues_show_id ON show_venues(show_id);
CREATE INDEX idx_show_venues_venue_id ON show_venues(venue_id);
CREATE INDEX idx_show_artists_show_id ON show_artists(show_id);
CREATE INDEX idx_show_artists_artist_id ON show_artists(artist_id);
CREATE INDEX idx_show_artists_position ON show_artists(show_id, position);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_is_active ON users(is_active);
CREATE INDEX idx_users_email_verified ON users(email_verified);

CREATE INDEX idx_oauth_accounts_user_id ON oauth_accounts(user_id);
CREATE INDEX idx_oauth_accounts_provider ON oauth_accounts(provider);
CREATE INDEX idx_oauth_accounts_provider_user_id ON oauth_accounts(provider_user_id);
CREATE INDEX idx_oauth_accounts_provider_email ON oauth_accounts(provider_email);

CREATE INDEX idx_user_preferences_user_id ON user_preferences(user_id);

-- Add comments for documentation
COMMENT ON TABLE artists IS 'Musical artists and bands';
COMMENT ON TABLE venues IS 'Concert venues and locations';
COMMENT ON TABLE shows IS 'Concert events and performances';
COMMENT ON TABLE show_artists IS 'Many-to-many relationship between shows and artists';
COMMENT ON TABLE users IS 'User accounts for authentication';
COMMENT ON COLUMN users.password_hash IS 'Bcrypt hashed password for local authentication';
COMMENT ON COLUMN users.email IS 'User email (unique, can be null for OAuth-only users)';
COMMENT ON COLUMN users.username IS 'User display name (unique, can be null for OAuth-only users)';
COMMENT ON TABLE oauth_accounts IS 'OAuth provider connections (Goth compatible)';
COMMENT ON COLUMN oauth_accounts.provider IS 'OAuth provider name (google, github, etc.)';
COMMENT ON COLUMN oauth_accounts.provider_user_id IS 'External provider user ID';
COMMENT ON TABLE user_preferences IS 'User preferences and settings';
