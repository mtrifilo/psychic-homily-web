-- Fix miscategorized tags: these were seeded as 'genre' but should be 'other'.
-- Safe to run in any environment — no-op if the tags don't exist or are already correct.

UPDATE tags SET category = 'other'
WHERE name = 'multi-venue' AND category = 'genre';

UPDATE tags SET category = 'other'
WHERE name = 'urban festival' AND category = 'genre';

UPDATE tags SET category = 'other'
WHERE name = 'shoegaze-revival-2026' AND category = 'genre';
