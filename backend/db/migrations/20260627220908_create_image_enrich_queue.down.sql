-- PSY-1247 down: drop the outbox table (its indexes drop with it).
DROP TABLE IF EXISTS image_enrich_queue;
