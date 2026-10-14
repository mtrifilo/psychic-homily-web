# PH street basemap (Atlas street view)

`ph-dark-basemap.json` and `ph-light-basemap.json` are the Psychic
Homily-branded street styles the Atlas globe crossfades into past the Black
Marble zoom range, one per app theme. Each is a complete, standalone
MapLibre GL style (loadable as-is in Maputnik for tuning); `phBasemap.ts`
adapts the one a map asks for into the fragment `GlobeCanvas` merges into
its inline style, applying the globe→street background fade.

The dark file is the hand-curated source. The light file is written from it
by `frontend/scripts/atlas-light-basemap.mjs` (see "Light variant" below);
never edit it by hand.

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
  muted browns on the theme's background (`#0d0805` dark, `#f4f1ea` paper)
  so the orange scene/venue markers stay the loudest thing on the map.
- **Dark palette**: anchored to the `.dark` tokens in
  `frontend/app/globals.css`: `--background #0d0805`, `--card #17100b`,
  `--border #221b15`, `--foreground #eee7d9`. Road fills are interpolations
  within that warm-dark family, lightening with importance; water is the one
  deliberate departure (near-black desaturated blue `#090c12`) so coastlines
  read against the warm land.
- **Light palette**: the light column of the palette table on the Figma
  board (Product Designs, Atlas page, PSY-1567 row `1973:75`, table
  `1973:76`), anchored to the `:root` tokens: background `--background
  #f4f1ea`; buildings `--card #faf7f0` with a `--secondary #e8e3d6` outline
  (`--border` reads too loud on paper); roads interpolated from the
  background toward `--border #cabe9f`, darkening with importance; water
  `#dbe4ea`, again the one cool departure; labels from `--muted-foreground
  #6b5e4f` to `--foreground #1a1714`, each on a halo of the ground it sits
  on (paper, or water for water names).
- **Token snapshots**: nothing reads these tokens at runtime (each style is
  a hand-copied snapshot), so `phBasemap.test.ts` pins each variant's
  anchors against its block of `globals.css` and fails if a theme repaint
  moves one. The map attribution restyle in `globals.css` reads the tokens
  directly (`--muted-foreground` on `--background`), so it follows the
  theme.
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
   reduction; do not re-adopt POIs/shields/sprites. Then rewrite the light
   file: `node scripts/atlas-light-basemap.mjs` (from `frontend/`). A layer
   added, dropped or recoloured in the dark file needs its light colours in
   `LIGHT_PAINT` (`scripts/lib/ph-light-basemap.mjs`) first; the script
   refuses a dark file the table no longer fits.
3. Re-probe the glyph server if label fonts break
   (`curl -sI "https://tiles.openfreemap.org/fonts/Noto%20Sans%20Regular/0-255.pbf"`).
4. Run `bun run test:run features/scenes`: `phBasemap.test.ts` pins, for
   both files, spec-validity, CSP-host agreement, the available-font set,
   the GlobeCanvas merge contract and the token anchors; and that the light
   file differs from the dark one only in colours and is what the script
   writes today.
5. If a host changes, update `connect-src` in `frontend/next.config.ts`
   (MapLibre fetches tiles AND glyphs via `fetch()` — connect-src, never
   img-src).

   **Check the RESOLVED tile host, not just the TileJSON host.** The style
   declares `https://tiles.openfreemap.org/planet`; the tiles it actually
   fetches come from the `tiles` array inside that TileJSON response. Those
   point at the same host today (verified 2026-07-26), and
   `phBasemap.test.ts` can only check the DECLARED host: it does no
   network I/O, deliberately, so the suite stays offline-safe. If OpenFreeMap
   ever moves delivery to a CDN subdomain, CSP would block tiles in
   production with nothing failing in CI. Re-check by hand whenever the
   provider changes anything:

   ```
   curl -s https://tiles.openfreemap.org/planet | head -c 200
   ```

## Light variant

- **Selection**: `GlobeCanvas` draws the variant of the app theme (the
  `<html>` `.dark` class next-themes sets). A theme change remounts the map
  under a new key, the same remove-and-create a hide and show runs, so a
  map's style and marker colours are fixed for its lifetime and the camera
  carries over through `atlasCamera.ts`. There is no `setStyle` path.
- **What stays dark**: the globe (the NASA night-lights raster, or the light
  globe look on phone panes) is dark in both themes, and so are the CSS
  starfield and halo behind it. Only the street basemap past the crossfade
  follows the theme. The `/venues` mini Atlas asks for the dark variant in
  both themes.
- **Crossfade**: the light variant's ground (`ph-background`) ramps its
  colour from the dark ground `#0d0805` at the crossfade start to paper at
  its end, under the same opacity ramp both variants use, so the night globe
  dissolves into a ground that brightens with it instead of paper showing
  through a still-dark raster.
- **Markers**: `MARKER_RAMPS` in `frontend/features/scenes/components/globeScale.ts`
  holds one colour ramp per theme for the scene dots and venue pins; the
  light one rings a selected mark in paper.
- **Bundle**: both style files are static imports of `phBasemap.ts`, so the
  light file ships in the same lazy chunks as the dark one (the Atlas canvas
  and the `/venues` mini Atlas), whichever theme is in force.

## Light globe overlays (phone panes)

On an Atlas pane narrower than 900px (`isAtlasPhonePane` in
`frontend/features/scenes/atlasViewport.ts`, the rule that also picks the
sheet layout) the globe shows the light look instead of the NASA GIBS
raster: a flat ocean and the Natural Earth land file
(`globe-land-110m.geojson`, prefetched before the map exists), with three
overlay files drawn on top. `globeSurface.ts` (in this directory) owns the
layers and the loading; `globePlaces.ts` (in this directory) holds the
place-label rules and `frontend/features/scenes/components/globePlaceLabels.ts`
draws the labels.

| File (`frontend/public/atlas/`) | Contents | Drawn as |
|---|---|---|
| `globe-country-lines-110m.geojson` | 1:110m country boundary lines | `globe-country-lines` line layer |
| `globe-state-lines-50m.geojson` | 1:50m state and province lines (United States, Canada, Australia, Brazil) | `globe-state-lines` line layer, under the country lines |
| `globe-places-110m.geojson` | 1:110m populated places (243), in label rank order | DOM markers in the app's mono face (no glyph download) |

All three come from `frontend/scripts/atlas-globe-overlays.mjs`, built from
the same pinned Natural Earth commit as the land file;
`frontend/scripts/lib/natural-earth.mjs` checks each input's sha256 before
anything is written.

- **Light look only.** On the full look (panes of 900px and wider) both line
  layers are hidden and no place label is mounted. The boundary and place
  files are requested only when a map shows the light look, so a map that
  only ever shows the full look never requests them.
- **Load trigger.** The boundary and place files are fetched only after the
  map's first full render: the first `render` event with the style and every
  visible source loaded. MapLibre's `load` is not used because it never fires
  on `/atlas` (the pulse rings change paint every frame). Each of these
  fetches has a 10 s deadline, body included.
- **Label budget.** Labels show from zoom 2 up to, not including, zoom 5.5,
  where the street crossfade starts. At most 12 show at the entry zoom
  (2.36, the default camera's), rising linearly to 20 at zoom 5 and holding
  there. The budget is the same at every pane size.
- **Collision order.** Scene labels and scene dots on the near side of the
  globe are placed first and never move. The controls drawn over the map come
  next: the search pill, Drift, the Genres chip, the My Scenes strip (its
  chips and its star), the credit when it is drawn, and any other element
  over the map that takes pointer events or is an icon. Place labels go last,
  in rank order: each is kept only if it sits wholly inside the pane and
  clears the scene marks, the controls and every place label kept before it,
  until the budget is spent. The controls' boxes are read from the DOM when
  the labels mount and again after a resize or a change in the chrome, never
  per frame; a change that moves a control lays the labels out again. While
  a modal elsewhere on the page has turned pointer events off, the boxes
  last read are kept, and they are read again once it turns them back on.
  The chrome is found by walking the map pane: a wrapper around controls
  must be `pointer-events: none` or have no area of its own, as the Atlas
  chrome's wrappers are, or its whole box counts as one control and blocks
  every label under it.
- **Failure paths.** A boundary file that fails, stalls past its deadline or
  is not a FeatureCollection is handed to the map as a URL, so MapLibre
  fetches it and a second failure reaches `basemapTelemetry.ts` as a source
  error. The place file is retried once; if that also fails, the map draws
  no place labels and `reportGlobePlacesFailure` reports it. Both paths use
  the same Sentry event (`Atlas basemap tile source failed`, at most once per
  source per page session, nothing while the browser reports itself
  offline) with two tags that tell them apart:
  - `basemap_source`: `globeStateLines` or `globeCountryLines` for a
    boundary file, `globePlaces` for the place file.
  - `basemap_status`: the HTTP status of a failed response; `0` for a
    network failure (for `globePlaces`, also a request past its deadline);
    `none` for a place file whose body was not a usable FeatureCollection,
    or a boundary error that carried no status.

  `basemap_host` is `same-origin` for all three.

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
   we use, adjust per the regeneration steps above; `phBasemap.test.ts`
   will catch host/CSP drift.
