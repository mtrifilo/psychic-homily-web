-- Reverse 20260502023010_add_network_id_to_radio_stations.up.sql
ALTER TABLE radio_stations DROP COLUMN IF EXISTS network_id;
