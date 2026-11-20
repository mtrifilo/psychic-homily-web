import type { APIClient } from "./api";
import * as display from "./display";
import { searchArtistsByName, normalizeForComparison } from "./duplicates";

/**
 * Flag-only QA check for the dry run: an artist name on a bill that reads like
 * something other than one act. Five rules, the same ones the catalogue audit
 * used to find non-artists already stored as artists:
 *
 *  - event-like wording: the whole words "live at", "plays", "present",
 *    "presents", "and friends", "night" or "tribute", or "& friends", or a
 *    spaced " ~ ";
 *  - the billed room's name (a leading "the" dropped, four or more characters)
 *    or its city (four or more characters), as whole words;
 *  - five or more words;
 *  - two or more names joined by "and", "&", "+", "w/" or a comma, each of
 *    which already exists as an artist;
 *  - the name equals a subtitle line of the source, given on the show as
 *    `source_subtitles`.
 *
 * Every rule also matches real acts ("Ganja White Night", "Our Last Night"),
 * which is why this never refuses and never rewrites: it prints the flag and
 * leaves the call to the reviewer, and it says so when a name could not be
 * checked against the catalogue.
 */

/** The batch key that carries a source's subtitle lines for a show. */
const SOURCE_SUBTITLES_KEY = "source_subtitles";

/** The part of a show plan this check reads. */
export interface ArtistNamePlan {
  input: { event_date: string; city?: string };
  artists: Array<{ id?: number; name: string; status?: "existing" | "new" }>;
  venues: Array<{ name: string; city?: string }>;
  valid: boolean;
  duplicate?: { isDuplicate: boolean };
}

export type ArtistNameRule = "event-wording" | "room-or-city" | "five-words" | "joins-artists" | "source-subtitle";

export interface ArtistNameFlag {
  rule: ArtistNameRule;
  detail: string;
}

/** What a name is checked against: the show's rooms and cities and its source's subtitle lines. */
export interface ArtistNameContext {
  rooms: string[];
  cities: string[];
  subtitles: string[];
}

/**
 * Lowercases and folds every run of characters that are not letters or digits
 * to one space, the way the audit's SQL compares names.
 */
export function foldName(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

const EVENT_WORDS = / (live at|plays|presents?|and friends|night|tribute) /;

/** The event-like wording a name carries, or null. */
export function eventWording(name: string): string | null {
  const m = ` ${foldName(name)} `.match(EVENT_WORDS);
  if (m) return m[1];
  if (/&\s*friends(?![\p{L}\p{N}])/iu.test(name)) return "& friends";
  if (/\s~\s/.test(name)) return " ~ ";
  return null;
}

/** Whether folded `phrase` occurs in folded `name` as whole words. */
function containsWords(name: string, phrase: string): boolean {
  return ` ${foldName(name)} `.includes(` ${phrase} `);
}

/** The room name or city a name contains as whole words, or null. */
export function roomOrCity(name: string, rooms: string[], cities: string[]): string | null {
  for (const room of rooms) {
    const folded = foldName(room).replace(/^the /, "");
    if (folded.length >= 4 && containsWords(name, folded)) return `the room's name "${room}"`;
  }
  for (const city of cities) {
    const folded = foldName(city);
    if (folded.length >= 4 && containsWords(name, folded)) return `the city "${city}"`;
  }
  return null;
}

/** How many words a name has once folded. */
export function wordCount(name: string): number {
  const folded = foldName(name);
  return folded === "" ? 0 : folded.split(" ").length;
}

const JOINERS = /\s+(?:and|&|\+|w\/)\s+|\s*,\s+/i;

/** The names a joined name is made of, or null when it joins fewer than two. */
export function joinedParts(name: string): string[] | null {
  const parts = name
    .split(JOINERS)
    .map((p) => p.trim())
    .filter((p) => p !== "");
  return parts.length >= 2 ? parts : null;
}

/** Whether a name equals one of the source's subtitle lines once folded. */
export function equalsSubtitle(name: string, subtitles: string[]): boolean {
  const folded = foldName(name);
  return folded !== "" && subtitles.some((line) => foldName(line) === folded);
}

/**
 * Every rule a name trips. `isArtist` answers whether a name already exists as
 * an artist; it is asked only about the parts of a joined name.
 */
export async function artistNameFlags(
  name: string,
  context: ArtistNameContext,
  isArtist: (part: string) => Promise<boolean>,
): Promise<ArtistNameFlag[]> {
  const flags: ArtistNameFlag[] = [];
  const wording = eventWording(name);
  if (wording) flags.push({ rule: "event-wording", detail: `event-like wording ("${wording.trim()}")` });
  const place = roomOrCity(name, context.rooms, context.cities);
  if (place) flags.push({ rule: "room-or-city", detail: `contains ${place}` });
  const words = wordCount(name);
  if (words >= 5) flags.push({ rule: "five-words", detail: `${words} words` });
  const parts = joinedParts(name);
  if (parts) {
    const quoted = parts.map((p) => `"${p}"`).join(", ");
    try {
      const known = await Promise.all(parts.map((p) => isArtist(p)));
      if (known.every(Boolean)) {
        flags.push({ rule: "joins-artists", detail: `joins ${parts.length} names that are each an artist (${quoted})` });
      }
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      flags.push({ rule: "joins-artists", detail: `joins ${parts.length} names (${quoted}); could not check whether each is an artist: ${reason}` });
    }
  }
  if (equalsSubtitle(name, context.subtitles)) {
    flags.push({ rule: "source-subtitle", detail: "equals a subtitle line of the source" });
  }
  return flags;
}

/** The source's subtitle lines a show carries, accepting one string or a list. */
export function sourceSubtitles(input: Record<string, unknown>): string[] {
  const value = input[SOURCE_SUBTITLES_KEY];
  if (typeof value === "string") return [value];
  if (Array.isArray(value)) return value.filter((v): v is string => typeof v === "string");
  return [];
}

/**
 * An `isArtist` lookup against the catalogue: a part counts when an artist's
 * name equals it exactly, ignoring case and accents. Each part is searched
 * once per run.
 */
export function catalogueArtistLookup(client: APIClient): (part: string) => Promise<boolean> {
  const cache = new Map<string, Promise<boolean>>();
  return (part: string) => {
    const key = normalizeForComparison(part);
    let found = cache.get(key);
    if (!found) {
      found = searchArtistsByName(client, part).then((results) =>
        results.some((r) => normalizeForComparison(r.name) === key),
      );
      cache.set(key, found);
    }
    return found;
  };
}

/**
 * Prints a flag for every artist name on a show the run would create that
 * trips a rule. Invalid and duplicate shows are skipped: nothing is created for
 * them. A joined name whose parts could not be looked up is flagged as
 * unchecked rather than passed.
 */
export async function reportArtistNameFlags(
  client: APIClient,
  plans: ArtistNamePlan[],
  isArtist: (part: string) => Promise<boolean> = catalogueArtistLookup(client),
): Promise<void> {
  const lines: string[] = [];
  for (let i = 0; i < plans.length; i++) {
    const plan = plans[i];
    if (!plan.valid || plan.duplicate?.isDuplicate) continue;
    const context: ArtistNameContext = {
      rooms: plan.venues.map((v) => v.name),
      cities: [plan.input.city, ...plan.venues.map((v) => v.city)].filter((c): c is string => !!c),
      subtitles: sourceSubtitles(plan.input as Record<string, unknown>),
    };
    const where = `${plan.input.event_date} at ${context.rooms.join(" + ")}`;
    for (const artist of plan.artists) {
      const which = artist.status === "existing" && artist.id !== undefined ? `existing artist #${artist.id}` : "new artist";
      const flags = await artistNameFlags(artist.name, context, isArtist);
      if (flags.length === 0) continue;
      lines.push(`[${i + 1}/${plans.length}] "${artist.name}" (${which}) on ${where}: ${flags.map((f) => f.detail).join("; ")}`);
    }
  }
  if (lines.length === 0) return;
  display.header("Artist names to check (flag only: nothing is refused or changed)");
  for (const line of lines) display.warn(line);
}
