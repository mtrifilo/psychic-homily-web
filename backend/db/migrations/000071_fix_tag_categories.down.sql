-- Revert tag category fixes: set back to 'genre'.

UPDATE tags SET category = 'genre'
WHERE name = 'multi-venue' AND category = 'other';

UPDATE tags SET category = 'genre'
WHERE name = 'urban festival' AND category = 'other';

UPDATE tags SET category = 'genre'
WHERE name = 'shoegaze-revival-2026' AND category = 'other';
