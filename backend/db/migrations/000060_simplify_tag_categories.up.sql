-- Migrate unused categories to 'other'
UPDATE tags SET category = 'other' WHERE category IN ('mood', 'era', 'style', 'instrument');
