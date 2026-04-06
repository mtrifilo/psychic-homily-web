CREATE TABLE radio_import_jobs (
    id SERIAL PRIMARY KEY,
    show_id INTEGER NOT NULL REFERENCES radio_shows(id),
    station_id INTEGER NOT NULL REFERENCES radio_stations(id),
    since DATE NOT NULL,
    until DATE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    episodes_found INTEGER NOT NULL DEFAULT 0,
    episodes_imported INTEGER NOT NULL DEFAULT 0,
    plays_imported INTEGER NOT NULL DEFAULT 0,
    plays_matched INTEGER NOT NULL DEFAULT 0,
    current_episode_date VARCHAR(10),
    error_log TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_radio_import_jobs_show_id ON radio_import_jobs(show_id);
CREATE INDEX idx_radio_import_jobs_status ON radio_import_jobs(status);
