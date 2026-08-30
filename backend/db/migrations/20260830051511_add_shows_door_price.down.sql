-- Rolling back drops every recorded door price. There is no second home for
-- the fact (it is not mirrored into `price`, which carries the advance price
-- on exactly these rows), so the values are gone. That is the accepted cost of
-- a reversible additive migration; `price` itself is untouched either way.
ALTER TABLE shows DROP COLUMN door_price;
