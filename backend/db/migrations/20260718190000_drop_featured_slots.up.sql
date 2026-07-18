-- PSY-1480: retire Featured Bill/Collection editorial slots.
-- Public consumer was removed with /explore → /graph (PSY-1457); admin
-- curation surface and endpoints are deleted in the same change.
DROP TABLE IF EXISTS featured_slots;
