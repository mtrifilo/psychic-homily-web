-- Reverse the timezone fix: shift back by 7 hours
UPDATE shows
SET event_date = event_date - INTERVAL '7 hours',
    updated_at = NOW()
WHERE id BETWEEN 667 AND 687;
