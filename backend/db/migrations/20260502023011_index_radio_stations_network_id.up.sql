CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_radio_stations_network_id ON radio_stations(network_id) WHERE network_id IS NOT NULL;
