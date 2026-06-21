/**
 * Inline label-roster expansion.
 *
 * A label page (e.g. a record label's "Artists" page) is naturally expressed as
 * one label plus a roster of artists. This module lets a single batch item carry
 * that roster inline:
 *
 *   { "entity_type": "label", "name": "Sacred Bones Records",
 *     "artists": [{ "name": "Anika" }, { "name": "Amen Dunes" }] }
 *
 * rather than repeating `"label": "Sacred Bones Records"` on every artist item.
 *
 * Expansion is pure sugar: it rewrites the inline form into the flat form the
 * rest of the pipeline already understands — the label item (minus `artists`)
 * plus one `artist` item per roster entry, each with `label` injected. The
 * existing batch processing order (labels before artists) then creates the label
 * first and links each artist to it via the existing `artist.label` path.
 */

export interface RosterItem {
  entity_type?: string;
  name?: unknown;
  artists?: unknown;
  label?: unknown;
  [key: string]: unknown;
}

export interface RosterExpansion {
  items: RosterItem[];
  /** How many label items carried a non-empty inline roster. */
  expandedLabels: number;
  /** How many artist items were produced from those rosters. */
  expandedArtists: number;
}

/**
 * Normalize one roster entry into a flat artist batch item.
 *
 * Accepts either a bare name string (`"Anika"`) or a full artist object
 * (`{ name, city, tags, ... }`). The label name is injected as `label` unless
 * the entry already specifies its own (an explicit override wins). Returns null
 * for empty/garbage entries so they're dropped rather than producing a nameless
 * artist. Object entries are passed through even without a `name` so downstream
 * artist validation surfaces the error instead of it disappearing silently.
 */
function toArtistItem(
  entry: unknown,
  labelName: string | undefined,
): RosterItem | null {
  if (typeof entry === "string") {
    const name = entry.trim();
    if (!name) return null;
    const item: RosterItem = { entity_type: "artist", name };
    if (labelName) item.label = labelName;
    return item;
  }

  if (entry && typeof entry === "object") {
    const obj = entry as Record<string, unknown>;
    const item: RosterItem = { ...obj, entity_type: "artist" };
    const hasOwnLabel = typeof item.label === "string" && item.label;
    if (!hasOwnLabel && labelName) {
      item.label = labelName;
    }
    return item;
  }

  return null;
}

/**
 * Expand any label items that carry an inline `artists` roster into a flat list
 * of label + artist items. Non-label items, and label items without a roster,
 * pass through unchanged. The `artists` key is always stripped from label items
 * so it never reaches the label API payload.
 */
export function expandInlineRosters(items: RosterItem[]): RosterExpansion {
  const out: RosterItem[] = [];
  let expandedLabels = 0;
  let expandedArtists = 0;

  for (const item of items) {
    const isLabel = item?.entity_type === "label";
    const roster = isLabel ? item.artists : undefined;

    if (isLabel && Array.isArray(roster) && roster.length > 0) {
      const labelName = typeof item.name === "string" ? item.name : undefined;
      const { artists: _omit, ...labelItem } = item;
      out.push(labelItem);

      for (const entry of roster) {
        const artistItem = toArtistItem(entry, labelName);
        if (artistItem) {
          out.push(artistItem);
          expandedArtists++;
        }
      }
      expandedLabels++;
    } else if (isLabel && "artists" in item) {
      // Empty or non-array `artists` — drop the key so it never hits the label API.
      const { artists: _omit, ...labelItem } = item;
      out.push(labelItem);
    } else {
      out.push(item);
    }
  }

  return { items: out, expandedLabels, expandedArtists };
}
