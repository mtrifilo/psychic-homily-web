# Label discography-page ingest

Flat **catalogue list** pages (often defunct labels) → label + artists + releases directly (no Bandcamp). Proven: **Creation Records** (`creation-records.com/discography/`, 104 artists + 325 singles, 2026-06-22).

## Parser

- **Format:** `CAT – Artist – Title` per line, `<br>`-separated (`CRE001 – The Legend! – '73 in '83`). Split on ` – ` (en-dash).
- **Parse BODY, NOT `<head>` JSON-LD/meta** — SEO plugins embed truncated run-together copy.
- **Exclude placeholders** — `CRE### – Not used`, trailing "THE END".

## Fields

- **`release_type`:** page framing (Creation `CRE` = singles → default `single`). Re-type `\bEP\b` titles → `ep` via `PUT /releases/{id}`.
- **`catalog_number`:** set `labels: ["<label>"]` + `catalog_number: "<CAT>"` (PSY-1183). Single-label only.
  - **Write-once default;** `--overwrite-catalog` or per-item `"overwrite_catalog_number": true` to correct (PSY-1194).
- **No year** when page omits it.
- **Label metadata** from known history (flag as such). Don't set `website` to fan-archive URL.
- **Collaborations kept whole.** Same person different credit → keep both; flag for merge.

## Workflow

1. Parse → unique artists + releases; label with inline `artists` + one `release` per row
2. **Artist-skip QA CRITICAL** — famous-label collisions at 0.6 fuzzy. Pre-create via `POST /admin/artists`
3. Dry-run → confirm → verify roster `count` + sample `GET /artists/{id}/releases`
4. **Don't register defunct labels** for refresh (frozen catalog)

See [troubleshooting.md](troubleshooting.md) for PSY-1184 release re-run idempotency.

## Label registry (discography pages)

| Label | Discography URL | Render | Notes |
| --- | --- | --- | --- |
| **Creation Records** | `https://creation-records.com/discography/` (fan archive) | Server-rendered (WordPress) — plain `curl`; the list is a `<br>`-separated `<p>` (parse the BODY, not the truncated `<head>` meta copy) | First run 2026-06-22 → **stage**: label id 4 (London, UK — from known history) + **104 artists + 325 singles**. The `CRE` catalogue = singles (albums are `CRELP`, not on this page). 8 "Not used" placeholders excluded; 18 EP-titled re-typed `single`→`ep`. Pre-created BMX Bandits / Momus / Phil Wilson / Silverfish / William to beat 0.6 fuzzy false-matches. Ed Ball/Edward Ball kept as 2 (same person, source credits both). No year. Catalogue numbers NOT yet backfilled — first run predated PSY-1183; backfill needs PSY-1184 deployed. Not registered for refresh (defunct = frozen). |
