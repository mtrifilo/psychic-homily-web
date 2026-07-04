# Label roster-page ingest

A label's "Artists" page is the label analog of a venue events page. Re-runs are idempotent (existing skip; label↔artist links `ON CONFLICT DO NOTHING`).

## Inline roster shape (preferred)

One self-contained label item with `artists` array:

```json
[
  {
    "entity_type": "label",
    "name": "Sacred Bones Records",
    "country": "US",
    "website": "https://sacredbonesrecords.com",
    "artists": [
      {"name": "Anika"},
      {"name": "Amen Dunes"}
    ]
  }
]
```

CLI expands to label + artists with `label` injected. Roster entries: bare strings or full objects. Flat form (separate label + artist items) still works.

Dry-run reports link plan; confirm reports outcomes.

## Workflow

1. **Render — `curl` first, browser only if names absent.** Feel It (Shopify) server-renders; Sacred Bones needs JS. Inspect markup each run.
2. **Honor page sections** — don't guess music vs non-music (Sacred Bones: Artists + Alumni kept, Books excluded).
3. **Build programmatically** — one label + inline `artists`. Trust slug over display typos. Capture per-artist Bandcamp when cards link externally (Feel It); internal `/collections/` = names only (Sacred Bones, Dais).
4. **Keep collaboration names un-split** — "Boris & Uniform" stays whole.
5. **Dry-run + artist-skip QA (MANDATORY)** — map every SKIP to proposed name; pre-create distinct artists via `POST /admin/artists` (not `ph submit artist`). Re-ingest enriches existing links (PSY-1171).
6. **Confirm + verify** — `GET /labels/{id}/artists` → check **`count`** (not `total`; no limit param). Verify links via `GET /artists/{id}` detail.
7. **Stamp source registry:**
   ```bash
   bun run src/entry.ts --env <env> sources register label <id> "<roster_url>"
   bun run src/entry.ts --env <env> sources refresh label <id>
   ```
   Stale list: `ph sources stale --limit 20`
8. **Optional follow:** [link-enrichment.md](link-enrichment.md) → [release-pass.md](release-pass.md) → [artist-tag-rollup.md](artist-tag-rollup.md)

> **Label `twitter`:** only `twitter.com`/`x.com` — Bluesky 422s. See [troubleshooting.md](troubleshooting.md).

> **PSY-1171:** re-ingest enriches artist/venue social links. Deferred (PSY-1179): label socials beyond website/bandcamp, venue address/zipcode/capacity.

## Label registry

| Label | Roster URL | Render | Sections (music kept / excluded) | Notes |
| --- | --- | --- | --- | --- |
| **Sacred Bones Records** | `https://www.sacredbonesrecords.com/pages/artists` | JS (Shopify) — browser MCP; artists are `/collections/<slug>` links under `#MainContent` | **Artists (80) + Alumni (50) = 130 kept**; **Books (12) excluded** (visual artists/authors: Peter Beste, Jesse Draxler, …) | First run 2026-06-20 → **stage**: label id 1 + 130 linked. Pre-created Institute / Lathe of Heaven / Cheena / Emma Ruth Rundle & Thou to beat 0.6 fuzzy false-matches. 2 source typos fixed via slug (Children's Hospital, Daily Void). `release_count` 0 (roster page has no release data). |
| **Feel It Records** | `https://www.feelitrecordshop.com/pages/artists` | **Server-rendered (Shopify)** — plain `curl -A "Mozilla/5.0"` returns the whole roster (no browser MCP). Roster is a single `<div class="artist_wrapper">` of `.artist_container` cards; per card the name is the text after `<br>` (also in `<a alt>`/`<img alt>` — cross-check, they agreed 86/86) and the `href` is a Bandcamp **album** link whose **subdomain is the artist's Bandcamp root** (`https://<slug>.bandcamp.com`). | **86 kept (single section, all music)**; no Alumni/Books/visual-artist section to exclude. | First run 2026-06-21 → **stage**: label id 2 (Cincinnati, OH — verify location from `/pages/hours-and-location`, the label relocated from Richmond, VA) + 86 linked, **each artist carrying its Bandcamp root** so the playable Bandcamp section renders. Pre-created Fan Club / It Thing / Spllit / Vacation (with bandcamp) to beat 0.6 fuzzy false-matches to Yot Club / Nothing / Split / Medication, and PATCHed bandcamp onto real dups Artificial Go (481) / Man-Eaters (1105) / Sweeping Promises (351). Keep `and`/`/` names whole (Fashion Pimps and the Glamazons, Green/Blue); "The Cowboy" (Cleveland, thecowboycle) ≠ "The Cowboys" (thecowboysnow) — distinct, the Bandcamp subdomain disambiguates. **Verify Bandcamp via `GET /artists/{id}` detail, NOT `GET /labels/{id}/artists`** — the roster *list projection* omits `social`/`bandcamp` (reads 0/86 falsely); detail confirmed 86/86. |
| **12XU** | `https://12xurecs.bandcamp.com/` (the label's **Bandcamp hub** — its root IS the "Artists \| 12XU" grid; the label's own site `12xu.net` is a WordPress blog, not a clean roster) | **Server-rendered (Bandcamp)** — plain `curl -A "Mozilla/5.0"`. **NEW source type: a Bandcamp *label* hub.** The `data-blob` only carries `{label_name, artist_grid:bool}` (no list) → parse the DOM grid: each `<li class='artists-grid-item'>` has `<a href='https://<sub>.bandcamp.com?…'>` (→ artist Bandcamp root, strip the `?` query), `<div class="artists-grid-name">` (name), `<div class="artists-grid-location secondaryText">` (City, State/Country). | **59 kept (single grid, all music)**; no non-music section. | First run 2026-06-21 → **stage**: label id 3 (Austin, TX — *inferred*: owner Gerard Cosloy's BC location + roster plurality; 12xu.net doesn't state it) + 59 linked, each with its `<x>12xu.bandcamp.com` root. **Captured per-artist city/state from the grid location** (US → 2-letter state via a full-name map; international → city only, no state, no locale tag; bare-country → neither). **Multi-label win:** Uniform (1717, already on Sacred Bones) also linked to 12XU — the cross-label graph enrichment. Pre-created chimers / Love Child / Rocket 808 (with bandcamp + location) to beat 0.6 fuzzy false-matches to Chambers / Wild Child / Rocket; PATCHed bandcamp onto existing exact-dups Uniform (1717) / The Sleeves (1492). Keep `&`/`/` names whole (Ed Kuepper & Jim White, Blank Hellscape / Wolf Eyes, USA/Mexico, John Schooley & Walter Daniels). |
| **Dais Records** | `https://www.daisrecords.com/pages/current` | **Server-rendered (Shopify)** — plain `curl -A "Mozilla/5.0"`. Three `<h1>`/`<h2>` sections: **Current Artists (30) + Re-Issues (17) + Alumni (44) = 89 unique** (2 cross-listed in Re-Issues + Alumni — dedupe once). Cards are `href="/collections/<slug>" title="Artist Name"` on `.artist-card` — **names only**, no per-artist Bandcamp on the roster page (same internal-collections pattern as Sacred Bones). Label socials from site footer (`daisrecords.bandcamp.com`, IG, YouTube); **omit Bluesky from `twitter`** (422 — see note above). | **All three music sections kept**; no separate non-music section. | First run 2026-07-03 → **stage**: label id 7 (Brooklyn, NY; founded 2007) + 89 linked. Pre-created Drew McDowall / SoiSong / Tempers / Whip & The Body to beat 0.6 fuzzy false-matches (Rose McDowall, Balisong, Temples, Uniform & The Body). **Link enrich:** label Bandcamp hub + MusicBrainz → **83/89** with links. **Release-pass:** 927 releases / 76 artists with BC discography; 501 created + 425 skipped. **Artist tag rollup:** 76/89 tagged. |
| **4AD** | `https://4ad.com/artists` | TBD — verify on first refresh | TBD | Ingested 2026-07 (label id 5, 22 artists) — add render notes on next run. |
| **Warp** | `https://warp.net/artists` | TBD — verify on first refresh | TBD | Ingested 2026-07 (label id 6, 146 artists) — add render notes on next run. |

## Typical pipeline

```
roster ingest → link enrichment → release-pass → artist tag rollup
```

Each step is optional except roster for new labels.
