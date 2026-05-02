-- Reverse 20260502023012_seed_wfmu_network_and_substreams.up.sql
DELETE FROM radio_stations WHERE slug IN ('wfmu-drummer', 'wfmu-rocknsoulradio', 'wfmu-sheena');

UPDATE radio_stations
   SET network_id = NULL
 WHERE slug = 'wfmu';

DELETE FROM radio_networks WHERE slug = 'wfmu';
