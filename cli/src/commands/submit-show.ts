import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateShow } from "../lib/schemas";
import { searchArtistsByName, searchVenuesByName, similarityScore, checkShowDuplicate } from "../lib/duplicates";
import type { EntitySearchResult, ShowDuplicateResult } from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import * as display from "../lib/display";
import { green, yellow, dim, gray } from "../lib/ansi";
import { resolveVenueTimezone, localTimeToUTC } from "../lib/timezone";
import { isValidSetType, statedSlot } from "../lib/setType";
import { resolveShowTimes } from "../lib/showTimes";
import type { ShowTimes } from "../lib/showTimes";

/** Whether a date string states a calendar day and no time of day. */
function isDateOnly(date: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(date);
}

/**
 * Normalize a date string to an ISO 8601 UTC timestamp.
 *
 * When only a date (YYYY-MM-DD) is provided, defaults to 20:00 local time.
 * When a date+time without timezone is provided, treats it as local time.
 * In both cases, converts from the venue's local timezone to UTC.
 *
 * @param date     - Date string (YYYY-MM-DD, YYYY-MM-DDTHH:MM, or full ISO 8601)
 * @param state    - Venue state, used only when no venue timezone is known
 * @param timezone - The matched venue's IANA timezone. This is the authority:
 *                   see resolveVenueTimezone (PSY-1873).
 */
export function normalizeDate(
  date: string,
  state?: string,
  timezone?: string,
): string {
  const zone = resolveVenueTimezone(state, timezone);

  // Date only: default to 20:00 local time
  if (isDateOnly(date)) {
    return localTimeToUTC(date, "20:00", zone);
  }

  // Date+time but no timezone suffix (Z or +/-offset): treat as local time
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/.test(date)) {
    const [datePart, timePart] = date.split("T");
    return localTimeToUTC(datePart, timePart, zone);
  }

  // Already has timezone info — return as-is
  return date;
}

// -- Types -------------------------------------------------------------------

interface ShowArtistInput {
  name: string;
  is_headliner?: boolean;
  /**
   * Curated bill role. Absent means the slot is unknown, which is what the API
   * records when the key is omitted; a default here would publish a guess.
   */
  set_type?: string;
}

interface ShowVenueInput {
  name: string;
  city?: string;
  state?: string;
  address?: string;
}

interface ShowInput {
  event_date: string;
  title?: string;
  city: string;
  state: string;
  /**
   * The advance price when a door price is also stated, otherwise the show's
   * only price.
   */
  price?: number;
  /**
   * The price at the door, carried ONLY when the source states one separately
   * from the advance price. Never derived from `price`.
   */
  door_price?: number;
  /**
   * The stated door time as a LOCAL WALL CLOCK ("7:00 PM"), not an instant. The
   * `shows.doors_at` column it feeds holds a UTC instant; the conversion is
   * buildShowPayload's, because only there is the venue's timezone known.
   */
  doors_at?: string;
  /** The stated music time, same shape and same conversion as `doors_at`. */
  music_at?: string;
  age_requirement?: string;
  description?: string;
  ticket_url?: string;
  artists: ShowArtistInput[];
  venues: ShowVenueInput[];
  tags?: TagInput[];
}

interface ResolvedArtist {
  id?: number;
  name: string;
  is_headliner?: boolean;
  set_type?: string;
  status: "existing" | "new";
  confidence?: number;
}

interface ResolvedVenue {
  id?: number;
  name: string;
  city?: string;
  state?: string;
  address?: string;
  /**
   * The matched venue's stored IANA timezone (PSY-1873). Present only for an
   * "existing" match. A venue this run is about to CREATE has no geocoded zone
   * yet, so its show falls back to the state map exactly as before. Never sent
   * in the payload: it is a property of the venue row, not of the show.
   */
  timezone?: string;
  status: "existing" | "new";
  confidence?: number;
}

export interface ShowPlan {
  input: ShowInput;
  artists: ResolvedArtist[];
  venues: ResolvedVenue[];
  valid: boolean;
  errors: string[];
  duplicate?: ShowDuplicateResult;
}

export interface SubmitShowsResult {
  plans: ShowPlan[];
  created: number;
  failed: number;
  skipped: number;
}

// -- Core logic (exported for testing) ---------------------------------------

/** Parse JSON input into an array of show objects. */
export function parseShowInput(jsonStr: string): ShowInput[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(jsonStr);
  } catch {
    throw new Error("Invalid JSON input");
  }

  if (Array.isArray(parsed)) {
    return parsed as ShowInput[];
  }
  return [parsed as ShowInput];
}

/** Resolve artists against the API, returning resolved entries with IDs when found. */
export async function resolveArtists(
  client: APIClient,
  artists: ShowArtistInput[],
): Promise<ResolvedArtist[]> {
  const resolved: ResolvedArtist[] = [];

  for (const artist of artists) {
    try {
      const results = await searchArtistsByName(client, artist.name);
      // Find best match by similarity score, require >= 0.7
      const best = results
        .map((r) => ({ ...r, score: similarityScore(artist.name, r.name) }))
        .filter((r) => r.score >= 0.7)
        .sort((a, b) => b.score - a.score)[0];

      if (best) {
        resolved.push({
          id: best.id,
          name: best.name,
          is_headliner: artist.is_headliner,
          set_type: artist.set_type,
          status: "existing",
          confidence: best.score,
        });
      } else {
        resolved.push({
          name: artist.name,
          is_headliner: artist.is_headliner,
          set_type: artist.set_type,
          status: "new",
        });
      }
    } catch {
      // If search fails, treat as new
      resolved.push({
        name: artist.name,
        is_headliner: artist.is_headliner,
        set_type: artist.set_type,
        status: "new",
      });
    }
  }

  return resolved;
}

/** Resolve venues against the API, returning resolved entries with IDs when found. */
export async function resolveVenues(
  client: APIClient,
  venues: ShowVenueInput[],
): Promise<ResolvedVenue[]> {
  const resolved: ResolvedVenue[] = [];

  for (const venue of venues) {
    try {
      const results = await searchVenuesByName(client, venue.name);
      // Find best match by similarity score, require >= 0.7.
      // The annotation keeps EntitySearchResult's index signature, which a bare
      // object spread would drop. `timezone` is only reachable through it.
      const best = results
        .map(
          (r): EntitySearchResult & { score: number } => ({
            ...r,
            score: similarityScore(venue.name, r.name),
          }),
        )
        .filter((r) => r.score >= 0.7)
        .sort((a, b) => b.score - a.score)[0];

      if (best) {
        resolved.push({
          id: best.id,
          name: best.name,
          city: venue.city,
          state: venue.state,
          address: venue.address,
          timezone:
            typeof best.timezone === "string" ? best.timezone : undefined,
          status: "existing",
          confidence: best.score,
        });
      } else {
        resolved.push({
          name: venue.name,
          city: venue.city,
          state: venue.state,
          address: venue.address,
          status: "new",
        });
      }
    } catch {
      resolved.push({
        name: venue.name,
        city: venue.city,
        state: venue.state,
        address: venue.address,
        status: "new",
      });
    }
  }

  return resolved;
}

/**
 * The door and music instants this plan will store, plus any note explaining a
 * stated time that is not being stored.
 *
 * Exported so the dry run reports exactly what the payload will carry rather
 * than a second derivation of it.
 */
export function planShowTimes(plan: ShowPlan): ShowTimes {
  return resolveShowTimes({
    eventDate: plan.input.event_date,
    doorsAt: plan.input.doors_at,
    musicAt: plan.input.music_at,
    state: planVenueState(plan),
    timezone: plan.venues[0]?.timezone,
  });
}

/**
 * The state whose zone anchors this show's wall clocks: the matched venue's,
 * else the one stated on the venue input, else the show's own.
 */
function planVenueState(plan: ShowPlan): string | undefined {
  return plan.venues[0]?.state || plan.input.venues[0]?.state || plan.input.state;
}

/** Build the API request body for creating a show. */
export function buildShowPayload(plan: ShowPlan): Record<string, unknown> {
  // Determine the zone the show's wall-clock time is expressed in.
  // The matched venue's own IANA zone is the authority; the state (first
  // venue's, else show-level) is the fallback for a venue we are about to
  // create. See resolveVenueTimezone (PSY-1873).
  const venueState = planVenueState(plan);
  const venueTimezone = plan.venues[0]?.timezone;

  const times = planShowTimes(plan);

  const payload: Record<string, unknown> = {
    // A stated music time IS the show's start, so it anchors event_date rather
    // than sitting beside a different one: the status stripe prints the start
    // time from event_date and MUSIC from music_at, and two clocks for one fact
    // is a contradiction on the page. The 20:00 default normalizeDate applies to
    // a date-only input is the convention for "no time known", so a known one
    // replaces it. An event_date that states its own time is left alone; the
    // caller stated both.
    event_date:
      times.musicAt !== undefined && isDateOnly(plan.input.event_date)
        ? times.musicAt
        : normalizeDate(plan.input.event_date, venueState, venueTimezone),
    city: plan.input.city,
    state: plan.input.state,
    artists: plan.artists.map((a) => {
      const artist: Record<string, unknown> = {};
      if (a.id) artist.id = a.id;
      if (!a.id) artist.name = a.name;
      // A stated role is authoritative and is_headliner is DERIVED from it, so
      // the two halves cannot contradict. An UNSTATED role leaves the key off,
      // which is the only way to tell the API the slot is unknown.
      const role =
        a.set_type !== undefined && isValidSetType(a.set_type)
          ? a.set_type
          : undefined;
      if (role) {
        artist.set_type = role;
        artist.is_headliner = role === "headliner";
      } else if (a.is_headliner !== undefined) {
        artist.is_headliner = a.is_headliner;
      }
      return artist;
    }),
    venues: plan.venues.map((v) => {
      const venue: Record<string, unknown> = {};
      if (v.id) venue.id = v.id;
      if (!v.id) {
        venue.name = v.name;
        if (v.city) venue.city = v.city;
        if (v.state) venue.state = v.state;
        if (v.address) venue.address = v.address;
      }
      return venue;
    }),
  };

  if (plan.input.title) payload.title = plan.input.title;
  // `!== undefined`, not truthiness: zero is a price the site prints as "Free".
  if (plan.input.price !== undefined) payload.price = plan.input.price;
  if (plan.input.door_price !== undefined) {
    payload.door_price = plan.input.door_price;
  }
  if (times.doorsAt !== undefined) payload.doors_at = times.doorsAt;
  if (times.musicAt !== undefined) payload.music_at = times.musicAt;
  if (plan.input.age_requirement) payload.age_requirement = plan.input.age_requirement;
  if (plan.input.description) payload.description = plan.input.description;
  if (plan.input.ticket_url) payload.ticket_url = plan.input.ticket_url;

  return payload;
}

/** Main entry point: validate, resolve, preview, and optionally submit shows. */
export async function submitShows(
  client: APIClient,
  jsonStr: string,
  confirm: boolean,
): Promise<SubmitShowsResult> {
  // 1. Parse input
  const shows = parseShowInput(jsonStr);

  // 2. Validate and resolve each show
  const plans: ShowPlan[] = [];

  for (const show of shows) {
    const validation = validateShow(show);
    if (!validation.valid) {
      plans.push({
        input: show,
        artists: [],
        venues: [],
        valid: false,
        errors: validation.errors.map((e) => `${e.field}: ${e.message}`),
      });
      continue;
    }

    // Resolve artists and venues against API
    const artists = await resolveArtists(client, show.artists);
    const venues = await resolveVenues(client, show.venues);

    // Check for duplicate shows (same date + venue + overlapping artist)
    const resolvedVenueIds = venues.filter((v) => v.id !== undefined).map((v) => v.id!);
    const resolvedArtistIds = artists.filter((a) => a.id !== undefined).map((a) => a.id!);
    const resolvedArtistNames = artists.map((a) => a.name);

    // Match buildShowPayload's timezone source so the dedup window aligns with
    // how event_date will be stored (venue-local evening → UTC).
    const venueState = venues[0]?.state || show.venues[0]?.state || show.state;
    const venueTimezone = venues[0]?.timezone;

    const duplicate = await checkShowDuplicate(
      client,
      show.event_date,
      resolvedVenueIds,
      resolvedArtistIds,
      resolvedArtistNames,
      venueState,
      venueTimezone,
    );

    plans.push({
      input: show,
      artists,
      venues,
      valid: true,
      errors: [],
      duplicate,
    });
  }

  // 2b. Resolve tags for all shows
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const plan of plans) {
    const tags = TagResolver.parseTags(plan.input.tags as TagInput[] | undefined);
    if (tags.length > 0 && plan.valid) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // 3. Display preview
  displayPreview(plans, resolvedTags);

  // 4. Summary
  const validPlans = plans.filter((p) => p.valid);
  const duplicatePlans = validPlans.filter((p) => p.duplicate?.isDuplicate);
  const creatablePlans = validPlans.filter((p) => !p.duplicate?.isDuplicate);
  const invalidCount = plans.length - validPlans.length;
  const duplicateCount = duplicatePlans.length;

  if (invalidCount > 0) {
    display.warn(`${invalidCount} show${invalidCount !== 1 ? "s" : ""} failed validation and will be skipped.`);
  }

  if (duplicateCount > 0) {
    for (const plan of duplicatePlans) {
      const label = plan.input.title || `${plan.input.event_date} show`;
      display.info(`EXISTING: ${label} (ID: ${plan.duplicate!.existingShowId}) — skipping`);
    }
  }

  if (creatablePlans.length === 0 && duplicateCount === 0) {
    display.error("No valid shows to submit.");
    return { plans, created: 0, failed: 0, skipped: plans.length };
  }

  if (creatablePlans.length === 0) {
    display.info(`All ${duplicateCount} valid show${duplicateCount !== 1 ? "s" : ""} already exist. Nothing to create.`);
    return { plans, created: 0, failed: 0, skipped: invalidCount + duplicateCount };
  }

  // 5. Submit if confirmed
  if (!confirm) {
    display.info(`Dry run: ${creatablePlans.length} show${creatablePlans.length !== 1 ? "s" : ""} would be created. Use --confirm to submit.`);
    // Report would-be-creates under `created` so the batch summary mirrors the
    // confirmed-run accounting and the other entity types (artists/venues/...);
    // only genuinely existing (duplicate) or invalid shows count as skipped.
    return { plans, created: creatablePlans.length, failed: 0, skipped: invalidCount + duplicateCount };
  }

  let created = 0;
  let failed = 0;

  for (const plan of creatablePlans) {
    const payload = buildShowPayload(plan);
    try {
      const result = await client.post<{ id: number; slug?: string }>("/shows", payload);
      created++;
      const label = plan.input.title || `${plan.input.event_date} show`;
      display.success(`Created: ${label} (ID: ${result.id})`);
      // Apply tags if any
      const parsedTags = TagResolver.parseTags(plan.input.tags as TagInput[] | undefined);
      if (result.id && parsedTags.length > 0) {
        const tagResult = await tagResolver.applyToEntity("show", result.id, parsedTags);
        if (tagResult.applied > 0) {
          display.info(`  Applied ${tagResult.applied} tag(s)`);
        }
      }
    } catch (err) {
      failed++;
      const label = plan.input.title || `${plan.input.event_date} show`;
      const message = err instanceof Error ? err.message : "Unknown error";
      display.error(`Failed to create ${label}: ${message}`);
    }
  }

  display.summary(created, 0, failed + invalidCount + duplicateCount);

  return { plans, created, failed, skipped: invalidCount + duplicateCount };
}

// -- Display helpers ---------------------------------------------------------

/**
 * One amount as the site spells it: `Free` for zero, `$20` for a whole number,
 * `$20.50` otherwise.
 *
 * The fourth statement of the site's money register, alongside `formatPrice` in
 * `frontend/lib/utils/formatters.ts`, `showPriceAmount` in
 * `backend/internal/services/shared/show_price.go` and `Show.formatPrice` in
 * `ios/PsychicHomily/Models/Show.swift`. No compiler holds the four together,
 * so a change here needs the same change in the other three. The whole-number
 * test is the part that drifts: rendering everything as whole dollars turns
 * $12.50 into `$12` and a fifty-cent door into `$0`.
 *
 * Anything that is not a finite number prints VERBATIM. The batch schema types
 * both price fields as number-or-string and nothing coerces them, so the
 * preview's job is to show the value that is about to be sent -- which the API
 * will reject -- rather than to throw and take the rest of the run with it.
 */
function formatAmount(amount: number): string {
  if (typeof amount !== "number" || !Number.isFinite(amount)) {
    return String(amount);
  }
  if (amount === 0) return "Free";
  return Number.isInteger(amount) ? `$${amount}` : `$${amount.toFixed(2)}`;
}

/**
 * The dry run's `Price` line: `$20`, `$25 door`, `$20 / $25 door`, or null when
 * the show states no price at all.
 *
 * A preview of the PAYLOAD, so both numbers are printed whenever both will be
 * sent, including two equal ones — a preview that collapsed them would hide a
 * duplicated number from the one reader who can still fix it before it is
 * written.
 *
 * Zero is a price, not silence, which is why the guards test `!== undefined`.
 * Neither half is ever inferred from the other.
 */
export function showPriceLine(input: {
  price?: number;
  door_price?: number;
}): string | null {
  const parts: string[] = [];
  if (input.price !== undefined) parts.push(formatAmount(input.price));
  if (input.door_price !== undefined) {
    parts.push(`${formatAmount(input.door_price)} door`);
  }
  return parts.length > 0 ? parts.join(" / ") : null;
}

/**
 * The dry run's per-act role tag: `[headliner]`, `[direct_support]`, or nothing.
 *
 * Formatting only. Which slot an act states is {@link statedSlot}'s question,
 * so `performer`, blank and out-of-vocabulary values are all silent here.
 */
export function billRoleTag(artist: { is_headliner?: boolean; set_type?: string }): string {
  const role = statedSlot(artist);
  return role ? dim(` [${role}]`) : "";
}

function displayPreview(plans: ShowPlan[], resolvedTags?: ResolvedTag[][]): void {
  for (let i = 0; i < plans.length; i++) {
    const plan = plans[i];
    const idx = plans.length > 1 ? ` [${i + 1}/${plans.length}]` : "";

    if (!plan.valid) {
      display.header(`Show${idx}: INVALID`);
      for (const err of plan.errors) {
        display.error(err);
      }
      continue;
    }

    const label = plan.input.title || `${plan.input.event_date} in ${plan.input.city}, ${plan.input.state}`;
    const dupTag = plan.duplicate?.isDuplicate
      ? ` ${green(`DUPLICATE (ID: ${plan.duplicate.existingShowId})`)}`
      : "";
    display.header(`Show${idx}: ${label}${dupTag}`);
    // Show the anchoring zone and the instant it produces, not just the date
    // the input stated. The zone silently decides what gets stored, and a
    // wrong one is only visible here as a UTC timestamp on the wrong day
    // (PSY-1873). "(from state)" flags that no venue zone was available, which
    // is the case worth a second look for a venue outside the US.
    const previewZone = resolveVenueTimezone(
      plan.venues[0]?.state || plan.input.venues[0]?.state || plan.input.state,
      plan.venues[0]?.timezone
    );
    const zoneSource = plan.venues[0]?.timezone ? "venue" : "from state";
    display.kv("Date", plan.input.event_date);
    display.kv(
      "Anchored",
      `${buildShowPayload(plan).event_date} ${gray(`(${previewZone}, ${zoneSource})`)}`
    );
    display.kv("Location", `${plan.input.city}, ${plan.input.state}`);

    // The stated wall clock next to the instant it becomes, for the same reason
    // the Anchored line prints both: the zone is what decides, and a wrong one
    // is only visible as a UTC timestamp. A stated time that is NOT being stored
    // says so here, since the preview is the only place a reader can still fix
    // it.
    const times = planShowTimes(plan);
    if (times.doorsAt !== undefined) {
      display.kv("Doors", `${plan.input.doors_at} ${gray(`-> ${times.doorsAt}`)}`);
    }
    if (times.musicAt !== undefined) {
      display.kv("Music", `${plan.input.music_at} ${gray(`-> ${times.musicAt}`)}`);
    }
    for (const note of times.notes) {
      display.warn(note);
    }

    const priceLine = showPriceLine(plan.input);
    if (priceLine) {
      display.kv("Price", priceLine);
    }
    if (plan.input.age_requirement) {
      display.kv("Ages", plan.input.age_requirement);
    }
    if (plan.input.ticket_url) {
      display.kv("Tickets", plan.input.ticket_url);
    }

    // Artists
    process.stderr.write(`\n  ${gray("Artists:")}\n`);
    for (const artist of plan.artists) {
      const confidenceStr = artist.confidence !== undefined && artist.confidence < 1.0
        ? ` ${(artist.confidence * 100).toFixed(0)}%`
        : "";
      const tag = artist.status === "existing"
        ? green(`EXISTING (ID: ${artist.id})${confidenceStr ? yellow(` [${confidenceStr} match]`) : ""}`)
        : yellow("NEW");
      process.stderr.write(`    ${artist.name} ${tag}${billRoleTag(artist)}\n`);
    }

    // Venues
    process.stderr.write(`\n  ${gray("Venues:")}\n`);
    for (const venue of plan.venues) {
      const confidenceStr = venue.confidence !== undefined && venue.confidence < 1.0
        ? ` ${(venue.confidence * 100).toFixed(0)}%`
        : "";
      const tag = venue.status === "existing"
        ? green(`EXISTING (ID: ${venue.id})${confidenceStr ? yellow(` [${confidenceStr} match]`) : ""}`)
        : yellow("NEW");
      process.stderr.write(`    ${venue.name} ${tag}\n`);
    }

    // Tags
    if (resolvedTags && resolvedTags[i].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[i]));
      for (const tag of resolvedTags[i]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
      }
    }
  }
}

// -- CLI runner (called from cli.ts) -----------------------------------------

export async function runSubmitShow(
  json: string | undefined,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  let jsonStr: string;

  if (json) {
    jsonStr = json;
  } else {
    // Read from stdin
    jsonStr = await readStdin();
    if (!jsonStr.trim()) {
      display.error("No JSON input provided. Pass JSON as argument or pipe via stdin.");
      process.exit(1);
    }
  }

  const client = new APIClient(env);
  const result = await submitShows(client, jsonStr, confirm);

  const hasDuplicates = result.plans.some((p) => p.duplicate?.isDuplicate);
  // `process.exitCode`, not `process.exit()`: shows written before a failure
  // still need their ISR revalidation flushed at the end of the run (PSY-1691).
  if (result.failed > 0 || (result.created === 0 && confirm && !hasDuplicates)) {
    process.exitCode = 1;
  }
}

async function readStdin(): Promise<string> {
  // Check if stdin has data (piped input)
  if (process.stdin.isTTY) {
    return "";
  }

  const chunks: Uint8Array[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(new Uint8Array(chunk));
  }
  return Buffer.concat(chunks).toString("utf-8");
}
