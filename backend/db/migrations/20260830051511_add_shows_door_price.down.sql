-- Rolling back drops every recorded door price. There is no second home for the
-- fact: it is NOT mirrored into `price`, which carries the advance price on
-- exactly these rows and is left untouched either way.
ALTER TABLE shows DROP COLUMN door_price;
