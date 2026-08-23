-- Account-level alert preferences: the user's home area and the alert matrix
-- every follow inherits from (PSY-1892 decisions 2, 4 and 9).
--
-- home_metro is the ONE metro the user calls home, stored as a US Census CBSA
-- code in the SAME code space as venues.metro / artists.metro so near-me alert
-- scoping compares codes directly instead of matching city strings (decision 7
-- rejects city equality). VARCHAR(10) mirrors those columns exactly. NULL means
-- no home area, which is the state that makes the near-me fallback in
-- engagement.EffectiveShowScope reachable.
--
-- alert_defaults is the account-level matrix, per alert type x channel:
--
--   {"shows":    {"in_app": true, "email": false},
--    "releases": {"in_app": true, "email": false}}
--
-- A nullable JSONB document rather than a grid of BOOLEAN columns because
-- ABSENT MUST BE REPRESENTABLE: absent means "inherit the shipped default"
-- (in-app ON, email OFF), and a bool column cannot say that. With booleans a
-- stored false would be indistinguishable from an unset key, so today's shipped
-- defaults would be frozen into every row at migration time and a later change
-- to them could never reach a user who had not touched the setting. It also
-- keeps the ON/OFF asymmetry out of GORM's zero-value trap, where a false on
-- Create is dropped in favour of the column default.
ALTER TABLE user_preferences
    ADD COLUMN home_metro VARCHAR(10),
    ADD COLUMN alert_defaults JSONB;
