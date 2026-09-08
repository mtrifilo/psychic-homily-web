-- PSY-1888: tags carry outbound links, so a crew tag page links out.

-- Three dedicated nullable columns rather than a JSONB links blob: the set is
-- fixed and small, and each column is judged by the rule the artist, venue,
-- label and festival columns of the SAME NAME are judged by. A JSONB key would
-- have to be re-checked against that registry on every read.
--
-- The two anchored columns are instagram and bandcamp: socialHostSuffixes in
-- internal/utils/url.go lists a host allowlist for each. website is absent from
-- that table on purpose and accepts any host; the scheme rule is all it gets.
--
-- The widths mirror the maxLength the tag request bodies declare, which mirror
-- urlFieldSpecs in internal/api/handlers/shared/url_validation.go: instagram
-- 255, bandcamp and website 500. Length is enforced by those request tags, not
-- by the social gate. TagLinks_ColumnWidthsMatchTheURLRegistry (in the tag
-- handler's integration suite) reads these widths back out of the migrated
-- database and holds them to the registry, so a cap raised in one place and not
-- the other fails there rather than as a Postgres 22001 on a rare admin write.
--
-- NULL means "no link". The write paths clear an empty value to NULL, so an
-- unset link has one spelling and `website IS NULL` is a reliable query.
--
-- Nullable columns with no default: no table rewrite, and no lock beyond the
-- brief ACCESS EXCLUSIVE the catalog update itself takes.

ALTER TABLE tags
    ADD COLUMN website VARCHAR(500),
    ADD COLUMN instagram VARCHAR(255),
    ADD COLUMN bandcamp VARCHAR(500);

COMMENT ON COLUMN tags.website IS
    'Outbound link for the thing the tag names, any host. NULL when unset.';
COMMENT ON COLUMN tags.instagram IS
    'Instagram URL, anchored to instagram.com by the write boundary. NULL when unset.';
COMMENT ON COLUMN tags.bandcamp IS
    'Bandcamp URL, anchored to bandcamp.com by the write boundary. NULL when unset.';
