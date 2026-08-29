# PH dark basemap (Atlas street view)

`ph-dark-basemap.json` is the Psychic Homily-branded dark street style the
Atlas globe crossfades into past the Black Marble zoom range. It is a
complete, standalone MapLibre GL style (loadable as-is in Maputnik for
tuning); `phBasemap.ts` adapts it into the fragment `GlobeCanvas` merges
into its inline style, applying the globe→street background fade.

This README lives here (not in `docs/`) deliberately: `docs/` is gitignored
in this repo, and the provenance/fallback notes below must ship with the
style file they describe.

## Provenance and design intent

- **Tiles**: [OpenFreeMap](https://openfreemap.org) public instance —
  OpenMapTiles-schema vector tiles from the `https://tiles.openfreemap.org/planet`
  TileJSON. Free, no API key, no usage caps; operator asks for (but does not
  require) an OpenFreeMap credit. OpenFreeMap states it does not log tile
  requests; note that tile fetches are still geo-correlated per user by
  nature (the third party could observe which city a visitor looks at).
- **Layer set**: hand-curated against the OpenMapTiles schema, using
  OpenFreeMap's Liberty style (`https://tiles.openfreemap.org/styles/liberty`)
  as the source-layer/filter reference. This is a deliberate *reduction* of
  Liberty (~111 layers → ~25): no POIs, no highway shields, no transit
  icons, no house numbers, no 3D buildings, no sprite at all — streets are
  muted browns on the app's `#0d0805` background so the orange scene/venue
  markers stay the loudest thing on the map.
- **Palette**: anchored to the `.dark` tokens in `frontend/app/globals.css` —
  `--background #0d0805`, `--card #17100b`, `--border #221b15`,
  `--foreground #eee7d9`. Road fills are interpolations within that warm-dark
  family; water is the one deliberate departure (near-black desaturated blue
  `#090c12`) so coastlines read against the warm land. `--muted-foreground
  #9c8c7c` is used by the attribution restyle in `globals.css`, not by the
  style itself. Nothing reads these tokens at runtime — the style is a
  hand-copied snapshot — so `phDarkBasemap.test.ts` pins each anchor against
  `globals.css` and fails if a theme repaint moves one.
- **Fonts**: OpenFreeMap's glyph server serves a fixed set; only
  `Noto Sans Regular/Bold/Italic` are available (verified by HTTP probe —
  `Space Mono`, `Roboto Mono`, etc. 404). The PH type direction
  (Space Mono for data) is approximated with Noto Sans plus letter-spacing
  and uppercase treatment on district labels. Scene labels are unaffected —
  they are DOM markers in the app font, not glyph-rendered symbols.

## Regeneration (if the upstream base changes)

There is no build step — the style is versioned source. If OpenFreeMap
changes its schema or endpoints:

1. Fetch the current Liberty base:
   `curl -s https://tiles.openfreemap.org/styles/liberty -o /tmp/liberty.json`
2. Diff its `sources`, `glyphs`, and the `source-layer`/`filter` pairs of the
   layer classes we use (`landcover`, `landuse`, `park`, `water`, `waterway`,
   `aeroway`, `transportation`, `transportation_name`, `building`,
   `boundary`, `water_name`, `place`) against `ph-dark-basemap.json`, and
   update filters/source-layers to match. Keep the palette and the layer
   reduction — do not re-adopt POIs/shields/sprites.
3. Re-probe the glyph server if label fonts break
   (`curl -sI "https://tiles.openfreemap.org/fonts/Noto%20Sans%20Regular/0-255.pbf"`).
4. Run `bun run test:run features/scenes` — `phDarkBasemap.test.ts` pins
   spec-validity, CSP-host agreement, the available-font set, and the
   GlobeCanvas merge contract.
5. If a host changes, update `connect-src` in `frontend/next.config.ts`
   (MapLibre fetches tiles AND glyphs via `fetch()` — connect-src, never
   img-src).

   **Check the RESOLVED tile host, not just the TileJSON host.** The style
   declares `https://tiles.openfreemap.org/planet`; the tiles it actually
   fetches come from the `tiles` array inside that TileJSON response. Those
   point at the same host today (verified 2026-07-26), and
   `phDarkBasemap.test.ts` can only check the DECLARED host — it does no
   network I/O, deliberately, so the suite stays offline-safe. If OpenFreeMap
   ever moves delivery to a CDN subdomain, CSP would block tiles in
   production with nothing failing in CI. Re-check by hand whenever the
   provider changes anything:

   ```
   curl -s https://tiles.openfreemap.org/planet | head -c 200
   ```

## Attribution requirements

- **OpenStreetMap (required, ODbL)**: the `openmaptiles` source carries
  `© OpenStreetMap contributors`; GlobeCanvas renders it via a non-compact
  `AttributionControl` (always visible, per OSM's attribution guidance),
  docked **bottom-left**. Not MapLibre's bottom-right default: the Atlas
  chrome docks `GenreLegend` there at `z-10` (the control's own stacking
  context tops out at `z-index: 2`) and `ScenePreviewPanel` covers the whole
  right edge whenever a scene is selected — either one hides the required
  credit. Don't move it back without re-checking both.
- **OpenFreeMap (requested)**: included in the same source attribution.
- **NASA GIBS (requested)**: carried by the `nightEarth` source GlobeCanvas
  registers from `nightEarthRaster.ts` ("Imagery courtesy NASA GIBS"); shown
  by the same control.

Do not restore `attributionControl: false` without replacing the OSM credit
somewhere equally visible.

## Fallback: self-hosted Protomaps (if OpenFreeMap degrades)

OpenFreeMap's public instance is best-effort with no SLA. If it degrades or
disappears, the self-host path is Protomaps — a single-file `.pmtiles`
planet extract on our own object storage, no tile server:

1. Download a planet build (~120 GB full planet, or a smaller regional
   extract) from https://maps.protomaps.com/builds/ — these use the same
   OpenMapTiles-compatible schema flavors; check the build's schema notes.
2. Upload the `.pmtiles` to R2/S3 with public range-read access (R2 has no
   egress fees, which is why it's the recommended bucket).
3. In the frontend, add the `pmtiles` protocol adapter (the `pmtiles` npm
   package) and register it before map construction:
   `maplibregl.addProtocol('pmtiles', new pmtiles.Protocol().tile)`, then
   point the `openmaptiles` source at
   `pmtiles://https://<bucket-host>/planet.pmtiles`.
4. Glyphs are served by OpenFreeMap today; for full independence also vendor
   the three Noto Sans PBF stacks (they are static files — copy them under
   `frontend/public/` like the vendored MapLibre worker) and point `glyphs`
   at the same-origin path.
5. Update `connect-src` in `next.config.ts` to the bucket host (and drop
   `tiles.openfreemap.org` once fully migrated).
6. If Protomaps' schema build diverges from OpenMapTiles in a source-layer
   we use, adjust per the regeneration steps above; `phDarkBasemap.test.ts`
   will catch host/CSP drift.
