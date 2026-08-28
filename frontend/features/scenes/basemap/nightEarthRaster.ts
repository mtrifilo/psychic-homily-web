/**
 * The Atlas globe's NASA GIBS night-lights raster (PSY-1537 spike, PSY-1543
 * handoff), stated in one place so the telemetry filter and the map that
 * registers the source cannot drift apart.
 *
 * This lives beside the vector basemap rather than inside GlobeCanvas because
 * basemapTelemetry must name the source it reports, and GlobeCanvas already
 * imports basemapTelemetry — putting the constants there would close an import
 * cycle. Same shape as phBasemap's exports, for the same reason.
 */

/**
 * The raster source id GlobeCanvas registers, and the one MapLibre merges onto
 * an error fired by that source as it bubbles to the map.
 *
 * Consumed by basemapTelemetry.ts (PSY-1936), which reports a failure of this
 * source once per page session under its own `basemap_source` tag, distinct
 * from the vector source's.
 */
export const NIGHT_EARTH_SOURCE_ID = 'nightEarth'

/**
 * NASA GIBS Black Marble (VIIRS 2016 composite) — the night-earth raster the
 * PSY-1537 spike verified. Note the {z}/{y}/{x} order (WMTS row-before-column)
 * and .png. Public-domain NASA imagery; the host is allowlisted in the CSP
 * connect-src (MapLibre fetches tiles via fetch(), not <img>).
 */
export const NIGHT_EARTH_TILES =
  'https://gibs.earthdata.nasa.gov/wmts/epsg3857/best/VIIRS_Black_Marble/default/2016-01-01/GoogleMapsCompatible_Level8/{z}/{y}/{x}.png'

/**
 * The host the raster is configured against.
 *
 * Unlike the vector source — a TileJSON endpoint whose tile URLs are chosen by
 * the provider at runtime — the GIBS template above IS the tile URL, so this
 * host is the one that actually serves every tile. basemapTelemetry uses it as
 * the `basemap_host` fallback when an error arrives without a URL of its own.
 * Written out rather than parsed from the template so the tag stays a literal
 * (no URL parsing inside an error handler); nightEarthRaster.test.ts asserts
 * the two still agree.
 */
export const NIGHT_EARTH_TILE_HOST = 'gibs.earthdata.nasa.gov'
