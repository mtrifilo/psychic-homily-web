-- Revert: subtract 7 hours to restore the old (incorrect) UTC values
UPDATE shows
SET event_date = event_date - INTERVAL '7 hours'
WHERE source = 'discovery';
