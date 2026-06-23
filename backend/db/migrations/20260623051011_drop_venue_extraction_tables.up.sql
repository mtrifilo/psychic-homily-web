-- PSY-1165: drop the legacy venue-extraction tables. The extraction pipeline
-- (SchedulerService / PipelineService / fetcher / VenueSourceConfigService) was
-- retired operationally in PSY-1158 and its backend code is removed in this
-- change. The down recreates both tables at their final pre-removal schema
-- (venue_source_configs after 000044 default-change + 000046 extraction_notes;
-- venue_extraction_runs as created in 000043) so the up->down->up round-trip
-- and the migration-chain reverse both succeed.
DROP TABLE IF EXISTS venue_extraction_runs;
DROP TABLE IF EXISTS venue_source_configs;
