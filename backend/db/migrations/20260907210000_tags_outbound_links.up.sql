-- PSY-1888: tags carry outbound links, so a crew tag page links out.

-- Three dedicated nullable columns rather than a JSONB links blob: the set is
-- fixed and small, each column is validated by the same per-platform host
-- allowlist the artist/venue/label/festival columns of the SAME NAME go through
-- (socialHostSuffixes in internal/utils/url.go), and a JSONB key would have to
-- be re-checked against that registry on every read.
--
-- The widths are the caps the API boundary enforces (urlFieldSpecs in
-- internal/api/handlers/shared/url_validation.go): instagram 255, bandcamp and
-- website 500. A column and the boundary that writes it must agree about what
-- fits, or an accepted value fails at the column as a 22001.
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
