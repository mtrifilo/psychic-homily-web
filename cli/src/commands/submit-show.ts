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
import { readEventDate, resolveShowTimes } from "../lib/showTimes";
import type { ShowTimes, ShowTimesRefusal } from "../lib/showTimes";

/**
 * Normalize a date string to an ISO 8601 UTC timestamp.
 *
 * A date with no time of day takes 20:00 venue-local, the repo's convention for
 * "the source stated a day and no clock". A date with a naive time takes that
 * time, read venue-local. A date that already states a zone names its own
 * instant and is passed through.
 *
 * Anything else is returned unchanged, which sends it to the API to be rejected
 * per show rather than guessed at here. `readEventDate` is deliberately strict
 * about the shapes above, so a value that reaches this branch is one no reader
 * in this repo can place on a calendar; the dry run prints it on the Anchored
 * line and `resolveShowTimes` refuses the show's times for the same reason.
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
  const reading = readEventDate(date, zone);
  if (reading === null) return date;

  switch (reading.kind) {
    case "day":
      return localTimeToUTC(reading.day, "20:00", zone);
    case "local":
      return localTimeToUTC(reading.day, reading.time, zone);
    case "instant":
      return date;
  }
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
  /**
   * The matched venue ROW's state, as opposed to `state` above, which is the
   * one the batch stated and is only used to create a venue. Present only for
   * an "existing" match, and empty-string when the row carries no state, which
   * is the value the read surfaces judge the show's zone on.
   */
  matchedState?: string;
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
          matchedState:
            typeof best.state === "string" ? best.state : undefined,
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
 * The zone inputs every clock on this show is read in.
 *
 * When the venue already exists, its OWN row decides, because that row is what
 * the read surfaces render from: `showTimingInput` in
 * frontend/features/shows/utils.ts passes `venue.state ?? show.state` and the
 * venue's `timezone` to the same resolution this writer uses. A stored empty
 * state is an answer, not a gap, and falling through it to the state the batch
 * happened to claim is how a venue gets written in a zone its page will not
 * print. Only a venue this run is about to CREATE has no row to read, and then
 * the stated location is all there is.
 */
export function venueZone(
  venues: ResolvedVenue[],
  input: { state: string; venues: ShowVenueInput[] },
): { state?: string; timezone?: string } {
  const matched = venues[0];
  if (matched?.id !== undefined) {
    // `??`, not `||`: a stored empty state is the row's answer and the read
    // surfaces honour it. Only an ABSENT one falls through, which is the same
    // gesture `showTimingInput` makes with `venue?.state ?? show.state`.
    return {
      state: matched.matchedState ?? input.state,
      timezone: matched.timezone,
    };
  }
  return { state: input.venues[0]?.state || input.state };
}

/** The zone a built plan's clocks are read in. */
function planVenueZone(plan: ShowPlan): { state?: string; timezone?: string } {
  return venueZone(plan.venues, plan.input);
}

/**
 * The door and music instants this plan will store, plus a refusal for every
 * stated time that is not being stored.
 *
 * Exported so the dry run reports exactly what the payload will carry rather
 * than a second derivation of it.
 */
export function planShowTimes(plan: ShowPlan): ShowTimes {
  const zone = planVenueZone(plan);
  return resolveShowTimes({
    eventDate: plan.input.event_date,
    statedDoors: plan.input.doors_at,
    statedMusic: plan.input.music_at,
    state: zone.state,
    timezone: zone.timezone,
  });
}

/**
 * The instant `event_date` stores.
 *
 * A stated music time IS the show's start, so it anchors the date rather than
 * sitting beside a different one: the status stripe prints the start time from
 * `event_date` and MUSIC from `music_at`
 * (`startTimeFactSegment` / `doorsMusicFactSegment` in
 * frontend/features/shows/components/showStatusStripeCopy.ts), and two clocks
 * for one fact is a contradiction on the page. The 20:00 normalizeDate applies
 * to a date-only input is the convention for "no time known", so a known one
 * replaces it. An `event_date` that states its own time is left alone: the
 * caller stated both.
 */
function showStartInstant(plan: ShowPlan, times: ShowTimes): string {
  const zone = planVenueZone(plan);
  const reading = readEventDate(
    plan.input.event_date,
    resolveVenueTimezone(zone.state, zone.timezone),
  );
  if (times.musicAt !== undefined && reading?.kind === "day") {
    return times.musicAt;
  }
  return normalizeDate(plan.input.event_date, zone.state, zone.timezone);
}

/** Build the API request body for creating a show. */
export function buildShowPayload(plan: ShowPlan): Record<string, unknown> {
  const times = planShowTimes(plan);

  const payload: Record<string, unknown> = {
    event_date: showStartInstant(plan, times),
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

/** Only the two columns this path fills, as the API serves them. */
interface StoredShowTimes {
  doors_at?: string | null;
  music_at?: string | null;
}

export interface TimeBackfill {
  doorsAt?: string;
  musicAt?: string;
}

/**
 * Which stated times may be written onto a show that already exists.
 *
 * THE INVARIANT: only a column the stored row leaves EMPTY is ever filled. A
 * stored time is the curated value and outranks a re-scrape, so it is never
 * overwritten and never cleared. Returns nothing when the row already answers
 * both questions.
 *
 * The pair is judged as it will END UP, stored plus incoming, because that is
 * what `validateShowTimeOrder` judges on the server
 * (`backend/internal/api/handlers/catalog/show.go`). Filling a doors time that
 * lands after the stored music time would be refused there, so it is not sent.
 */
export function timesToBackfill(
  stored: StoredShowTimes,
  times: ShowTimes,
): TimeBackfill | null {
  const fill: TimeBackfill = {};
  if (!stored.doors_at && times.doorsAt !== undefined) fill.doorsAt = times.doorsAt;
  if (!stored.music_at && times.musicAt !== undefined) fill.musicAt = times.musicAt;
  if (fill.doorsAt === undefined && fill.musicAt === undefined) return null;

  const doors = fill.doorsAt ?? stored.doors_at ?? undefined;
  const music = fill.musicAt ?? stored.music_at ?? undefined;
  if (doors && music && Date.parse(music) < Date.parse(doors)) return null;

  return fill;
}

/**
 * Fill the door and music times an already-existing show is missing.
 *
 * A re-ingest of a calendar that has since published its times is the only way
 * a show created before them ever gets them, and the create path cannot help:
 * the show is a duplicate and is skipped. See `timesToBackfill` for what may be
 * written.
 */
export async function backfillShowTimes(
  client: APIClient,
  showId: number,
  times: ShowTimes,
): Promise<TimeBackfill | null> {
  const stored = await client.get<StoredShowTimes>(`/shows/${showId}`);
  const fill = timesToBackfill(stored ?? {}, times);
  if (fill === null) return null;

  const body: Record<string, unknown> = {};
  if (fill.doorsAt !== undefined) body.doors_at = fill.doorsAt;
  if (fill.musicAt !== undefined) body.music_at = fill.musicAt;
  await client.put(`/shows/${showId}`, body);
  return fill;
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

    // The dedup window and the instant this show will be written at must be
    // read in ONE zone, or the window brackets a day the row does not land on
    // and the duplicate is missed. `venueZone` is that one source; see its doc
    // for why a matched venue's own row wins over the state the batch claimed.
    const zone = venueZone(venues, show);

    const duplicate = await checkShowDuplicate(
      client,
      show.event_date,
      resolvedVenueIds,
      resolvedArtistIds,
      resolvedArtistNames,
      zone.state,
      zone.timezone,
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
      const id = plan.duplicate!.existingShowId!;
      display.info(`EXISTING: ${label} (ID: ${id}) — skipping`);

      const times = planShowTimes(plan);
      const states = times.doorsAt !== undefined || times.musicAt !== undefined;
      if (!states) continue;

      if (!confirm) {
        display.info(`  times stated; --confirm fills only the columns show ${id} leaves empty`);
        continue;
      }
      try {
        const filled = await backfillShowTimes(client, id, times);
        if (filled === null) {
          display.info(`  EXISTING: times not backfilled (show ${id} already has them)`);
        } else {
          const parts = [
            filled.doorsAt ? `doors_at ${filled.doorsAt}` : null,
            filled.musicAt ? `music_at ${filled.musicAt}` : null,
          ].filter(Boolean);
          display.success(`  Backfilled ${parts.join(", ")} on show ${id}`);
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unknown error";
        display.error(`  Failed to backfill times on show ${id}: ${message}`);
      }
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

/**
 * A source-supplied string, made safe to print in a preview a person approves.
 *
 * Batch values are extracted by a model from scraped third-party pages, so they
 * are attacker-influenced text on its way to a terminal. Control characters are
 * removed because a cursor-movement escape can rewrite lines already printed for
 * an EARLIER show, and the length is capped because an unbounded value scrolls
 * the rest of the batch out of the scrollback. Either one lets the preview
 * describe something other than what the confirmed run will send.
 *
 * Scoped to the values this command interpolates. The same treatment belongs on
 * every source-derived string `display` prints, which is a wider change than
 * this one.
 */
function quoteSourceValue(value: string): string {
  const stripped = value.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, "");
  return stripped.length > 120 ? `${stripped.slice(0, 120)}...` : stripped;
}

/**
 * One line saying which stated time is not being stored, and why.
 *
 * The wording lives here rather than in `resolveShowTimes` because it is dry-run
 * copy: the rule is the library's, the sentence is this command's.
 */
export function describeShowTimeRefusal(refusal: ShowTimesRefusal): string {
  switch (refusal.reason) {
    case "no-timezone":
      return "doors/music times not stored: no timezone is known for this venue, so the clock would be anchored on the America/Phoenix default";
    case "no-calendar-day":
      return `doors/music times not stored: "${quoteSourceValue(refusal.eventDate)}" does not name a calendar day to anchor them to`;
    case "unreadable-music":
      return `music time not stored: "${quoteSourceValue(refusal.music)}" is not a readable time, so no doors time is stored either`;
    case "doors-without-music":
      return `doors time "${quoteSourceValue(refusal.doors)}" not stored: the source states no music time, and doors alone is half a schedule`;
    case "unreadable-doors":
      return `doors time not stored: "${quoteSourceValue(refusal.doors)}" is not a readable time`;
    case "music-before-doors":
      return `doors/music times not stored: music at "${quoteSourceValue(refusal.music)}" is before doors at "${quoteSourceValue(refusal.doors)}", which states a day this listing did not`;
    case "clock-does-not-exist": {
      const also = refusal.alsoDropped
        ? `, which also drops the readable doors time "${quoteSourceValue(refusal.alsoDropped)}"`
        : "";
      return `time not stored: "${quoteSourceValue(refusal.clock)}" is an hour that does not exist on ${refusal.day} in this venue's timezone, because the clocks moved forward through it${also}`;
    }
    case "small-hours-music":
      return `doors/music times not stored: a music time of "${quoteSourceValue(refusal.music)}" before ${refusal.cutoff} on a date-only listing does not say which day it belongs to, so event_date keeps the 20:00 convention rather than moving back to it`;
  }
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
    const zone = planVenueZone(plan);
    const previewZone = resolveVenueTimezone(zone.state, zone.timezone);
    // `ph batch` creates venues before shows, so a venue named in this same file
    // does not exist during the dry run and DOES during the confirmed one, where
    // its freshly geocoded row supplies a different zone. Saying which of the
    // two this line is reading is what keeps the preview honest.
    const zoneSource = zone.timezone
      ? "venue"
      : plan.venues[0]?.id === undefined
        ? "from state, venue not created yet"
        : "from state";
    const payload = buildShowPayload(plan);
    display.kv("Date", quoteSourceValue(String(plan.input.event_date)));
    display.kv(
      "Anchored",
      `${quoteSourceValue(String(payload.event_date))} ${gray(`(${previewZone}, ${zoneSource})`)}`
    );
    display.kv("Location", `${plan.input.city}, ${plan.input.state}`);

    // The stated wall clock next to the instant it becomes, for the same reason
    // the Anchored line prints both: the zone is what decides, and a wrong one
    // is only visible as a UTC timestamp. A stated time that is NOT being stored
    // says so here, since the preview is the only place a reader can still fix
    // it.
    const times = planShowTimes(plan);
    // Same `(zone, source)` suffix the Anchored line carries: these instants are
    // decided by the zone, and for a venue this run has not created yet the zone
    // the confirmed run uses is not the one shown here.
    const zoneNote = gray(`(${previewZone}, ${zoneSource})`);
    if (plan.duplicate?.isDuplicate) {
      // This show is not being created, so these instants are not what the run
      // writes. What it may do instead is fill columns the stored row lacks,
      // which is reported per show in the duplicate summary.
      if (times.doorsAt !== undefined || times.musicAt !== undefined) {
        display.kv("Times", "existing show; only absent columns can be filled");
      }
    } else if (times.doorsAt !== undefined) {
      display.kv(
        "Doors",
        `${quoteSourceValue(String(plan.input.doors_at))} ${gray(`-> ${times.doorsAt}`)} ${zoneNote}`,
      );
    }
    if (!plan.duplicate?.isDuplicate && times.musicAt !== undefined) {
      display.kv(
        "Music",
        `${quoteSourceValue(String(plan.input.music_at))} ${gray(`-> ${times.musicAt}`)} ${zoneNote}`,
      );
    }
    if (!plan.duplicate?.isDuplicate) {
      for (const refusal of times.refusals) {
        display.warn(describeShowTimeRefusal(refusal));
      }
    }
    // event_date only yields to music_at when it states no time of its own, so
    // an input that states both can put two different clocks on one show.
    if (times.musicAt !== undefined && payload.event_date !== times.musicAt) {
      display.warn(
        `event_date states its own time, so the show starts at ${payload.event_date} while music_at says ${times.musicAt}; the page will print both`,
      );
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
