-- PSY-1199: reverse the artist_link_suggestions queue. DROP TABLE removes the
-- table's index, CHECK/UNIQUE constraints, and FKs with it, so the
-- up->down->up CI round-trip lands back on the pre-PSY-1199 schema exactly.

DROP TABLE IF EXISTS artist_link_suggestions;
