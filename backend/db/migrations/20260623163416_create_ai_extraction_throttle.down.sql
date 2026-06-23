-- PSY-855: reverse the AI-extraction throttle table. DROP TABLE removes the
-- PK and FK constraint with it, so the up->down->up CI round-trip lands back
-- on the pre-PSY-855 schema exactly.

DROP TABLE IF EXISTS ai_extraction_throttle;
