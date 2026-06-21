-- PSY-1149: reverse the source-config registry. DROP TABLE removes the table's
-- index and CHECK/UNIQUE constraints with it, so the up->down->up CI round-trip
-- lands back on the pre-PSY-1149 schema exactly.

DROP TABLE IF EXISTS source_configs;
