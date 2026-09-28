-- When the viewer dismissed the one-time hint that follows their first saved
-- show. NULL means never dismissed, the only state in which the hint may
-- render. A nullable timestamp rather than a boolean so NULL is an honest
-- "not yet" that no zero-valued write can produce by accident.
--
-- Account-level on purpose: the hint is once per person across every device,
-- which browser storage cannot express.
ALTER TABLE user_preferences
    ADD COLUMN first_save_hint_dismissed_at TIMESTAMPTZ;
