/**
 * Genre-family taxonomy for the Atlas globe dot tint (PSY-1315) — the FRONTEND
 * half of a cross-layer contract. The backend (catalog genre-family map) owns the
 * tag-slug -> family rollup and the confident-dominance test, and emits a scene's
 * `dominant_genre` family KEY; this file owns family -> color + legend label.
 *
 * The KEYS below MUST match the `genreFamily*` constants in
 * backend/internal/services/catalog/genre_families.go — a key the backend emits
 * that is missing here leaves the dot un-tinted with no legend entry.
 *
 * Color is a fixed `--chart-1..8` slot per family (reusing the PSY-1083 graph
 * palette, colorblind-safe). `--chart-1` is a warm orange close to the no-data
 * DOT_COLOR_BASE, so exactly one family must take it — the RAREST one
 * (jazz_experimental), minimizing how often a tinted dot could be mistaken for an
 * untinted one. Every other family gets a distinctly non-orange slot.
 */

import {
  clusterColor,
  type GraphPalette,
} from '@/components/graph/graphPalette'

export interface GenreFamily {
  /** Stable key; mirrors the backend family constant. */
  key: string
  /** Legend label. */
  label: string
  /** `--chart-{colorIndex+1}` palette slot (PSY-1083). */
  colorIndex: number
}

// Order = legend order. colorIndex is the palette slot, NOT the array position:
// every family gets a distinctly non-orange slot EXCEPT the rarest
// (jazz_experimental), which takes the warm chart-1 nearest the no-data orange.
// See the file doc.
export const GENRE_FAMILIES: readonly GenreFamily[] = [
  { key: 'punk_hardcore', label: 'Punk & Hardcore', colorIndex: 3 }, // chart-4 (red)
  { key: 'rock_indie', label: 'Rock & Indie', colorIndex: 2 }, // chart-3 (gold)
  { key: 'electronic', label: 'Electronic', colorIndex: 5 }, // chart-6 (blue)
  { key: 'metal', label: 'Metal', colorIndex: 1 }, // chart-2 (green)
  { key: 'hip_hop', label: 'Hip-Hop & Rap', colorIndex: 4 }, // chart-5 (tan)
  { key: 'folk_country', label: 'Folk & Country', colorIndex: 6 }, // chart-7 (purple)
  { key: 'jazz_experimental', label: 'Jazz & Experimental', colorIndex: 0 }, // chart-1 (warm) — rarest, on the ambiguous slot
  { key: 'pop_soul', label: 'Pop, R&B & Soul', colorIndex: 7 }, // chart-8 (teal)
]

const FAMILY_BY_KEY: ReadonlyMap<string, GenreFamily> = new Map(
  GENRE_FAMILIES.map((f) => [f.key, f]),
)

/**
 * Resolved hex for a scene's dominant genre family, for the WebGL globe dot (the
 * canvas needs concrete hex, not a `var()` token). Returns undefined for an
 * absent or unknown key so the caller falls back to the neutral base color.
 */
export function genreFamilyColor(
  palette: GraphPalette,
  key: string | undefined | null,
): string | undefined {
  if (!key) return undefined
  const family = FAMILY_BY_KEY.get(key)
  if (!family) return undefined
  return clusterColor(palette, family.colorIndex)
}
