-- When the one-time hint that follows the viewer's first saved show was first
-- shown to them. The client sets it as the hint opens (and again, keeping the
-- first time, when it is closed). NULL means never shown, the only state in
-- which the hint may render. A nullable timestamp rather than a boolean so
-- NULL is an honest "not yet" that no zero-valued write can produce by
-- accident.
--
-- Account-level on purpose: the hint is once per person across every device,
-- which browser storage cannot express.
ALTER TABLE user_preferences
    ADD COLUMN first_save_hint_dismissed_at TIMESTAMPTZ;
