-- Rollback show reports feature

-- Drop show_reports table
DROP TABLE IF EXISTS show_reports;

-- Drop enum types
DROP TYPE IF EXISTS show_report_status;
DROP TYPE IF EXISTS show_report_type;
