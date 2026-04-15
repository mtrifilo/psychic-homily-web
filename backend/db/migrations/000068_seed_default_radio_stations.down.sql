-- Remove seeded radio shows (cascade from station delete won't apply here since
-- stations may have been created by the seed command before this migration existed)
DELETE FROM radio_shows WHERE slug IN (
    'the-morning-show', 'the-midday-show', 'the-afternoon-show',
    'audioasis', 'el-sonido', 'midnight-in-a-perfect-world',
    'trouble-wfmu', 'the-best-show-wfmu', 'bodega-pop-live-wfmu', 'downtown-soulville-wfmu',
    'floating-points-nts', 'charlie-bones-nts', 'brownswood-basement-nts', 'anu-nts'
);

DELETE FROM radio_stations WHERE slug IN ('kexp', 'wfmu', 'nts-radio');
