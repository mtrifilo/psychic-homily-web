import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseShowInput,
  resolveArtists,
  resolveVenues,
  buildShowPayload,
  planShowTimes,
  venueZone,
  describeShowTimeRefusal,
  timesToBackfill,
  backfillShowTimes,
  normalizeDate,
  showPriceLine,
  billRoleTag,
  submitShows,
  type ShowPlan,
} from "../src/commands/submit-show";
import { APIClient } from "../src/lib/api";
import { checkShowDuplicate, showDedupWindow } from "../src/lib/duplicates";
import { validateShow } from "../src/lib/schemas";
import type { ShowTimesRefusal } from "../src/lib/showTimes";

// -- Mock helpers ------------------------------------------------------------

function createMockClient(overrides: {
  get?: (path: string, params?: Record<string, string>) => Promise<unknown>;
  post?: (path: string, body?: unknown) => Promise<unknown>;
} = {}): APIClient {
  const client = Object.create(APIClient.prototype) as APIClient;

  if (overrides.get) {
    (client as unknown as Record<string, unknown>).get = overrides.get;
  } else {
    (client as unknown as Record<string, unknown>).get = async () => ({});
  }

  if (overrides.post) {
    (client as unknown as Record<string, unknown>).post = overrides.post;
  } else {
    (client as unknown as Record<string, unknown>).post = async () => ({ id: 1 });
  }

  return client;
}

/** A Chicago show at a venue whose own zone is known, plus whatever times. */
function planWithTimes(times: { doors_at?: string; music_at?: string }): ShowPlan {
  return {
    input: {
      event_date: "2026-09-04",
      city: "Chicago",
      state: "IL",
      ...times,
      artists: [{ name: "Wolves of Glendale" }],
      venues: [{ name: "Lincoln Hall", city: "Chicago", state: "IL" }],
    },
    artists: [{ id: 1, name: "Wolves of Glendale", status: "existing" }],
    venues: [
      {
        id: 2,
        name: "Lincoln Hall",
        state: "IL",
        timezone: "America/Chicago",
        status: "existing",
      },
    ],
    valid: true,
    errors: [],
  };
}

// -- parseShowInput ----------------------------------------------------------

describe("parseShowInput", () => {
  test("parses a single show object", () => {
    const input = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = parseShowInput(input);
    expect(result).toHaveLength(1);
    expect(result[0].event_date).toBe("2026-04-15");
    expect(result[0].artists[0].name).toBe("Nina Hagen");
  });

  test("parses an array of shows", () => {
    const input = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Tucson",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "191 Toole", city: "Tucson", state: "AZ" }],
      },
    ]);

    const result = parseShowInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].city).toBe("Phoenix");
    expect(result[1].city).toBe("Tucson");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseShowInput("not json")).toThrow("Invalid JSON input");
  });
});

// -- resolveArtists ----------------------------------------------------------

describe("resolveArtists", () => {
  test("marks artist as existing when found in search", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/artists/search")) {
          return {
            artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }],
          };
        }
        return {};
      },
    });

    const result = await resolveArtists(client, [{ name: "Nina Hagen" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("existing");
    expect(result[0].id).toBe(42);
    expect(result[0].name).toBe("Nina Hagen");
  });

  test("marks artist as new when not found in search", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [] }),
    });

    const result = await resolveArtists(client, [{ name: "Unknown Band" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
    expect(result[0].name).toBe("Unknown Band");
    expect(result[0].id).toBeUndefined();
  });

  test("preserves is_headliner flag", async () => {
    const client = createMockClient({
      get: async () => ({
        artists: [{ id: 1, name: "Headliner", slug: "headliner" }],
      }),
    });

    const result = await resolveArtists(client, [
      { name: "Headliner", is_headliner: true },
    ]);
    expect(result[0].is_headliner).toBe(true);
  });

  test("treats search failure as new artist", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await resolveArtists(client, [{ name: "Test" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
  });
});

// -- resolveVenues -----------------------------------------------------------

describe("resolveVenues", () => {
  test("marks venue as existing when found in search", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/venues/search")) {
          return {
            venues: [
              { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
            ],
          };
        }
        return {};
      },
    });

    const result = await resolveVenues(client, [
      { name: "Crescent Ballroom", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("existing");
    expect(result[0].id).toBe(10);
  });

  test("marks venue as new when not found", async () => {
    const client = createMockClient({
      get: async () => ({ venues: [] }),
    });

    const result = await resolveVenues(client, [
      { name: "New Venue", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
    expect(result[0].name).toBe("New Venue");
  });

  test("treats search failure as new venue", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await resolveVenues(client, [
      { name: "Test Venue", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
  });

  // PSY-1873: the matched venue's geocoded zone is the whole reason a non-US
  // show lands on the right instant, so it has to survive venue resolution.
  test("carries the matched venue's timezone", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/venues/search")) {
          return {
            venues: [
              {
                id: 160,
                name: "Boom Leeds",
                slug: "boom-leeds-leeds-england",
                city: "Leeds",
                state: "England",
                country: "United Kingdom",
                timezone: "Europe/London",
              },
            ],
          };
        }
        return {};
      },
    });

    const result = await resolveVenues(client, [
      { name: "Boom Leeds", city: "Leeds", state: "England" },
    ]);
    expect(result[0].status).toBe("existing");
    expect(result[0].timezone).toBe("Europe/London");
  });

  test("leaves timezone undefined for a venue with none stored", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/venues/search")) {
          return {
            venues: [
              { id: 29, name: "Berghain", slug: "berghain-berlin-de", city: "Berlin", state: "DE", timezone: null },
            ],
          };
        }
        return {};
      },
    });

    const result = await resolveVenues(client, [
      { name: "Berghain", city: "Berlin", state: "DE" },
    ]);
    expect(result[0].timezone).toBeUndefined();
  });

  test("leaves timezone undefined for a venue this run will create", async () => {
    const client = createMockClient({ get: async () => ({ venues: [] }) });

    const result = await resolveVenues(client, [
      { name: "Brand New Room", city: "Leeds", state: "England" },
    ]);
    expect(result[0].status).toBe("new");
    expect(result[0].timezone).toBeUndefined();
  });
});

// -- buildShowPayload --------------------------------------------------------

describe("buildShowPayload", () => {
  test("builds payload with existing artist ID", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      artists: [{ id: 42, name: "Nina Hagen", status: "existing" }],
      venues: [{ id: 10, name: "Crescent Ballroom", state: "AZ", status: "existing" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    // 8pm Phoenix (UTC-7) = 3am UTC next day
    expect(payload.event_date).toBe("2026-04-16T03:00:00Z");
    expect(payload.city).toBe("Phoenix");

    const artists = payload.artists as Array<Record<string, unknown>>;
    expect(artists[0].id).toBe(42);
    expect(artists[0].name).toBeUndefined();

    const venues = payload.venues as Array<Record<string, unknown>>;
    expect(venues[0].id).toBe(10);
    expect(venues[0].name).toBeUndefined();
  });

  test("anchors a non-US date-only show in the venue's own timezone", () => {
    // PSY-1873, end to end through the writer: the Leeds show that shipped as
    // 2026-10-24T03:00:00Z (20:00 America/Phoenix) is now 20:00 Europe/London.
    const plan: ShowPlan = {
      input: {
        event_date: "2026-10-23",
        city: "Leeds",
        state: "England",
        artists: [{ name: "Din of Celestial Birds" }],
        venues: [{ name: "Boom Leeds", city: "Leeds", state: "England" }],
      },
      artists: [{ id: 3411, name: "Din of Celestial Birds", status: "existing" }],
      venues: [
        {
          id: 160,
          name: "Boom Leeds",
          state: "England",
          timezone: "Europe/London",
          status: "existing",
        },
      ],
      valid: true,
      errors: [],
    };

    expect(buildShowPayload(plan).event_date).toBe("2026-10-23T19:00:00Z");
  });

  test("falls back to the state map when the venue has no timezone", () => {
    // Unchanged behaviour for a venue this run is about to create. Wrong for a
    // non-US room, but it is the only information the writer has, and the
    // backfill CLI re-anchors the row once the venue is geocoded.
    const plan: ShowPlan = {
      input: {
        event_date: "2026-10-23",
        city: "Leeds",
        state: "England",
        artists: [{ name: "Din of Celestial Birds" }],
        venues: [{ name: "Brand New Room", city: "Leeds", state: "England" }],
      },
      artists: [{ id: 3411, name: "Din of Celestial Birds", status: "existing" }],
      venues: [{ name: "Brand New Room", state: "England", status: "new" }],
      valid: true,
      errors: [],
    };

    expect(buildShowPayload(plan).event_date).toBe("2026-10-24T03:00:00Z");
  });

  test("builds payload with new artist name", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "New Band" }],
        venues: [{ name: "New Venue", city: "Phoenix", state: "AZ" }],
      },
      artists: [{ name: "New Band", status: "new" }],
      venues: [{ name: "New Venue", city: "Phoenix", state: "AZ", status: "new" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    const artists = payload.artists as Array<Record<string, unknown>>;
    expect(artists[0].name).toBe("New Band");
    expect(artists[0].id).toBeUndefined();

    const venues = payload.venues as Array<Record<string, unknown>>;
    expect(venues[0].name).toBe("New Venue");
    expect(venues[0].city).toBe("Phoenix");
    expect(venues[0].state).toBe("AZ");
  });

  /** A minimal valid plan carrying only the prices under test. */
  function planWithPrices(prices: {
    price?: number;
    door_price?: number;
  }): ShowPlan {
    return {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
        ...prices,
      },
      artists: [{ name: "Test", status: "new" }],
      venues: [{ name: "Test Venue", status: "new" }],
      valid: true,
      errors: [],
    };
  }

  test("includes optional fields when provided", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        title: "Special Show",
        price: 25,
        age_requirement: "21+",
        description: "A great show",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
      artists: [{ name: "Test", status: "new" }],
      venues: [{ name: "Test Venue", status: "new" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    expect(payload.title).toBe("Special Show");
    expect(payload.price).toBe(25);
    expect(payload.age_requirement).toBe("21+");
    expect(payload.description).toBe("A great show");
  });

  test("carries an advance/door pair through to the payload", () => {
    const payload = buildShowPayload(planWithPrices({ price: 20, door_price: 25 }));
    expect(payload.price).toBe(20);
    expect(payload.door_price).toBe(25);
  });

  test("a lone stated price never grows a door price", () => {
    const payload = buildShowPayload(planWithPrices({ price: 20 }));
    expect(payload.price).toBe(20);
    expect("door_price" in payload).toBe(false);
  });

  test("carries a door-only price with no advance price", () => {
    const payload = buildShowPayload(planWithPrices({ door_price: 25 }));
    expect(payload.door_price).toBe(25);
    expect("price" in payload).toBe(false);
  });

  test("carries a free show's zero rather than dropping it", () => {
    const payload = buildShowPayload(planWithPrices({ price: 0 }));
    expect(payload.price).toBe(0);
  });

  // -- doors_at / music_at ---------------------------------------------------


  test("converts a stated pair to venue-local instants", () => {
    const payload = buildShowPayload(
      planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" }),
    );
    // 2026-09-04 is CDT, UTC-5.
    expect(payload.doors_at).toBe("2026-09-05T00:30:00Z");
    expect(payload.music_at).toBe("2026-09-05T01:30:00Z");
  });

  test("a stated music time anchors event_date, so the two agree", () => {
    const payload = buildShowPayload(
      planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" }),
    );
    expect(payload.event_date).toBe(payload.music_at);
  });

  test("a date-only show with no stated music time keeps the 20:00 convention", () => {
    const payload = buildShowPayload(planWithTimes({}));
    expect(payload.event_date).toBe("2026-09-05T01:00:00Z");
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
  });

  test("an event_date that states its own time is not moved by music_at", () => {
    const plan = planWithTimes({ music_at: "8:30PM" });
    plan.input.event_date = "2026-09-04T21:00";
    const payload = buildShowPayload(plan);
    expect(payload.event_date).toBe("2026-09-05T02:00:00Z");
    expect(payload.music_at).toBe("2026-09-05T01:30:00Z");
  });

  test("a doors-only listing writes neither column", () => {
    const payload = buildShowPayload(planWithTimes({ doors_at: "7:30PM" }));
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
  });

  test("a contradictory pair writes neither column", () => {
    const payload = buildShowPayload(
      planWithTimes({ doors_at: "11:00 PM", music_at: "12:00 AM" }),
    );
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
  });

  test("an unreadable door time leaves the readable music time standing", () => {
    const payload = buildShowPayload(
      planWithTimes({ doors_at: "doors at 7", music_at: "8:30PM" }),
    );
    expect("doors_at" in payload).toBe(false);
    expect(payload.music_at).toBe("2026-09-05T01:30:00Z");
  });

  test("an existing venue is judged on its own row, not on the state the batch claims", () => {
    // The row is what every read surface renders from. A stored empty state with
    // no geocoded zone means the page refuses to print a clock, so writing one
    // here would store an instant nothing shows.
    const plan = planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" });
    plan.venues[0].timezone = undefined;
    plan.venues[0].matchedState = "";

    const payload = buildShowPayload(plan);
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
    expect(planShowTimes(plan).refusals).toEqual([{ reason: "no-timezone" }]);
  });

  test("an existing venue's own state anchors the clock when the batch disagrees", () => {
    const plan = planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" });
    plan.venues[0].timezone = undefined;
    plan.venues[0].matchedState = "AZ";
    plan.input.state = "IL";

    // 8:30 PM Phoenix (UTC-7 year round), not 8:30 PM Chicago.
    expect(buildShowPayload(plan).music_at).toBe("2026-09-05T03:30:00Z");
  });

  test("a venue this run will create falls back to the stated location", () => {
    const plan = planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" });
    plan.venues[0] = { name: "Lincoln Hall", state: "IL", status: "new" };

    expect(buildShowPayload(plan).music_at).toBe("2026-09-05T01:30:00Z");
  });

  test("a venue with no resolvable zone writes neither column and says why", () => {
    const plan = planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" });
    plan.input.state = "England";
    plan.input.venues[0].state = "England";
    plan.venues[0].matchedState = "England";
    plan.venues[0].timezone = undefined;

    const payload = buildShowPayload(plan);
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
    expect(planShowTimes(plan).refusals).toEqual([{ reason: "no-timezone" }]);
  });
});

describe("showPriceLine", () => {
  test("prints the advance/door split", () => {
    expect(showPriceLine({ price: 20, door_price: 25 })).toBe("$20 / $25 door");
  });

  test("prints a lone advance price bare", () => {
    expect(showPriceLine({ price: 20 })).toBe("$20");
  });

  test("labels a door-only price", () => {
    expect(showPriceLine({ door_price: 25 })).toBe("$25 door");
  });

  test("prints an equal pair as both numbers, matching the payload", () => {
    expect(showPriceLine({ price: 20, door_price: 20 })).toBe("$20 / $20 door");
  });

  test("prints a zero price as Free rather than reading it as silence", () => {
    expect(showPriceLine({ price: 0 })).toBe("Free");
  });

  test("spells a fractional amount to the cent", () => {
    expect(showPriceLine({ price: 20.5, door_price: 25 })).toBe(
      "$20.50 / $25 door",
    );
  });

  test("is null when the show states no price", () => {
    expect(showPriceLine({})).toBeNull();
  });

  test("prints a non-numeric price verbatim rather than throwing", () => {
    // The batch schema types both price fields as number-or-string and nothing
    // coerces, so a preview that threw here would abort a partially-written run.
    const stringy = { price: "$20", door_price: "25" } as unknown as {
      price?: number;
      door_price?: number;
    };
    expect(showPriceLine(stringy)).toBe("$20 / 25 door");
  });

  test("prints a NaN price verbatim rather than throwing", () => {
    expect(showPriceLine({ price: Number.NaN })).toBe("NaN");
  });
});

describe("billRoleTag", () => {
  // The tag is dimmed, and `dim` wraps whenever stdout is a TTY or FORCE_COLOR
  // is set. Asserting the raw string would make these the only tests in the
  // suite that fail under an interactive `bun test`.
  const plain = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, "");

  // The dry run is the last place an operator can catch a role that got
  // extracted wrong, so the preview has to print the one that will be written.
  test("prints a stated role", () => {
    expect(plain(billRoleTag({ set_type: "direct_support" }))).toBe(" [direct_support]");
  });

  test("prints a stated headliner once, not twice", () => {
    // is_headliner is derived from the role on the way in, so both signals
    // agree here and the tag must not double up.
    expect(plain(billRoleTag({ set_type: "headliner", is_headliner: true }))).toBe(
      " [headliner]",
    );
  });

  test("says nothing for an act with no stated slot", () => {
    expect(billRoleTag({})).toBe("");
  });

  test("says nothing for performer, which is a spelling of slot unknown", () => {
    expect(billRoleTag({ set_type: "performer" })).toBe("");
    expect(billRoleTag({ set_type: "   " })).toBe("");
  });

  test("says nothing for a role the API would refuse", () => {
    // The same value the edit commands drop rather than send back.
    expect(billRoleTag({ set_type: "co-headliner" })).toBe("");
  });

  test("falls back to the legacy flag when no role is stated", () => {
    expect(plain(billRoleTag({ is_headliner: true }))).toBe(" [headliner]");
  });
});

describe("bill roles on the create path", () => {
  function planWithArtist(artist: Record<string, unknown>): ShowPlan {
    return {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [artist as never],
        venues: [{ name: "Test Venue" }],
      },
      artists: [{ id: 42, name: "Test", status: "existing", ...artist } as never],
      venues: [{ name: "Test Venue", status: "new" }],
      valid: true,
      errors: [],
    };
  }

  test("carries a stated role and derives is_headliner from it", () => {
    const artists = buildShowPayload(
      planWithArtist({ name: "Test", set_type: "direct_support" }),
    ).artists as Array<Record<string, unknown>>;
    expect(artists[0].set_type).toBe("direct_support");
    expect(artists[0].is_headliner).toBe(false);
  });

  test("derives is_headliner true from a stated headliner role", () => {
    const artists = buildShowPayload(
      planWithArtist({ name: "Test", set_type: "headliner" }),
    ).artists as Array<Record<string, unknown>>;
    expect(artists[0].is_headliner).toBe(true);
  });

  test("leaves the key off an act that states no role", () => {
    const artists = buildShowPayload(
      planWithArtist({ name: "Test" }),
    ).artists as Array<Record<string, unknown>>;
    expect("set_type" in artists[0]).toBe(false);
  });

  test("a stated role outranks the legacy flag", () => {
    const artists = buildShowPayload(
      planWithArtist({ name: "Test", set_type: "opener", is_headliner: true }),
    ).artists as Array<Record<string, unknown>>;
    expect(artists[0].set_type).toBe("opener");
    expect(artists[0].is_headliner).toBe(false);
  });

  test("an out-of-vocabulary role fails validation instead of being dropped", () => {
    const result = validateShow({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test", set_type: "co-headliner" }],
      venues: [{ name: "Test Venue" }],
    });
    expect(result.valid).toBe(false);
    expect(result.errors[0].field).toBe("artists[0].set_type");
    expect(result.errors[0].message).toContain("direct_support");
  });

  test("a valid role passes validation", () => {
    expect(
      validateShow({
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Test", set_type: "dj" }],
        venues: [{ name: "Test Venue" }],
      }).valid,
    ).toBe(true);
  });
});

// -- submitShows (integration) -----------------------------------------------

describe("submitShows", () => {
  test("single show with existing artist and venue (resolved by search)", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      return {};
    };

    const postMock = async (_path: string, body?: unknown) => {
      return { id: 100, slug: "2026-04-15-crescent-ballroom" };
    };

    const client = createMockClient({ get: getMock, post: postMock });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.failed).toBe(0);
    expect(result.plans[0].artists[0].status).toBe("existing");
    expect(result.plans[0].artists[0].id).toBe(42);
    expect(result.plans[0].venues[0].status).toBe("existing");
    expect(result.plans[0].venues[0].id).toBe(10);
  });

  test("show with new artist (not found in search)", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [] }; // Not found
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 101 }),
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Brand New Band" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].artists[0].status).toBe("new");
    expect(result.plans[0].artists[0].name).toBe("Brand New Band");
  });

  test("tour announcement: array of shows with shared artist, different venues", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return { venues: [] }; // All venues new for simplicity
      }
      return {};
    };

    let postCount = 0;
    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 200 + ++postCount }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Tucson",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "191 Toole", city: "Tucson", state: "AZ" }],
      },
      {
        event_date: "2026-04-17",
        city: "Flagstaff",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "The Orpheum", city: "Flagstaff", state: "AZ" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(3);
    expect(result.failed).toBe(0);
    expect(result.plans).toHaveLength(3);
    // All share same artist resolved as existing
    for (const plan of result.plans) {
      expect(plan.artists[0].status).toBe("existing");
      expect(plan.artists[0].id).toBe(42);
    }
  });

  test("validation error: missing event_date", async () => {
    const client = createMockClient();

    const json = JSON.stringify({
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.plans[0].valid).toBe(false);
    expect(result.plans[0].errors.some((e) => e.includes("event_date"))).toBe(true);
  });

  test("dry-run mode: does not call POST", async () => {
    let postCalled = false;
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => {
        postCalled = true;
        return { id: 1 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, false); // dry-run
    // Dry-run reports the would-be-created show under `created` (not `skipped`),
    // mirroring the confirmed-run accounting and the other batch entity types.
    expect(result.created).toBe(1);
    expect(result.skipped).toBe(0);
    expect(postCalled).toBe(false);
  });

  test("confirm mode: calls POST and reports success", async () => {
    let postCalled = false;
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => {
        postCalled = true;
        return { id: 99 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true); // confirm
    expect(result.created).toBe(1);
    expect(postCalled).toBe(true);
  });

  test("handles API error during creation", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => { throw new Error("Server error"); },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.failed).toBe(1);
  });

  test("mixed valid and invalid shows in array", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => ({ id: 1 }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
      {
        // Missing event_date, city, state
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].valid).toBe(true);
    expect(result.plans[1].valid).toBe(false);
  });

  test("dry-run with a valid + invalid show: invalid counts as skipped", async () => {
    // Exercises the dry-run branch's `skipped: invalidCount + duplicateCount`:
    // the creatable show reports under `created` (1) and the invalid one under
    // `skipped` (1). Reaching this branch requires creatablePlans.length > 0,
    // so a valid+invalid mix is the discriminating case. (Pre-fix, the creatable
    // show was reported as skipped and `created` was 0.)
    let postCalled = false;
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => {
        postCalled = true;
        return { id: 1 };
      },
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
      {
        // Missing event_date, city, state -> invalid
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
    ]);

    const result = await submitShows(client, json, false); // dry-run
    expect(result.created).toBe(1);
    expect(result.skipped).toBe(1);
    expect(postCalled).toBe(false);
  });
});

// -- checkShowDuplicate ------------------------------------------------------

describe("checkShowDuplicate", () => {
  test("returns no match when no venue IDs provided", async () => {
    const client = createMockClient();
    const result = await checkShowDuplicate(client, "2026-04-15", [], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when no artist IDs or names provided", async () => {
    const client = createMockClient();
    const result = await checkShowDuplicate(client, "2026-04-15", [10], [], []);
    expect(result.isDuplicate).toBe(false);
  });

  test("detects duplicate by matching venue ID and artist ID", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 500,
              slug: "2026-04-15-crescent-ballroom",
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(500);
    expect(result.existingShowSlug).toBe("2026-04-15-crescent-ballroom");
  });

  test("detects duplicate by matching venue ID and artist name (fuzzy)", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 501,
              slug: "2026-04-15-crescent-ballroom",
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 99, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    // Artist IDs don't match (different ID), but names match
    const result = await checkShowDuplicate(client, "2026-04-15", [10], [200], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(501);
  });

  test("returns no match when venue does not match", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 502,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 99, name: "Different Venue" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when artist does not match", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 503,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 99, name: "Totally Different Band" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when no shows exist on that date", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when API call fails", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("handles full ISO date strings", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 504,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15T20:00:00Z", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(504);
  });
});

// -- submitShows with deduplication ------------------------------------------

describe("submitShows deduplication", () => {
  test("skips duplicate show in confirm mode", async () => {
    let postCalled = false;
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return [
          {
            id: 500,
            slug: "2026-04-15-crescent-ballroom",
            event_date: "2026-04-15T20:00:00Z",
            venues: [{ id: 10, name: "Crescent Ballroom" }],
            artists: [{ id: 42, name: "Nina Hagen" }],
          },
        ];
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => {
        postCalled = true;
        return { id: 999 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.skipped).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(result.plans[0].duplicate?.existingShowId).toBe(500);
    expect(postCalled).toBe(false);
  });

  test("skips duplicate show in dry-run mode", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return [
          {
            id: 500,
            slug: "2026-04-15-crescent-ballroom",
            event_date: "2026-04-15T20:00:00Z",
            venues: [{ id: 10, name: "Crescent Ballroom" }],
            artists: [{ id: 42, name: "Nina Hagen" }],
          },
        ];
      }
      return {};
    };

    const client = createMockClient({ get: getMock });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, false);
    expect(result.created).toBe(0);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
  });

  test("creates new show when no duplicate found", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return []; // No existing shows on this date
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 100 }),
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(false);
  });

  test("mixed batch: one duplicate, one new", async () => {
    const getMock = async (path: string, params?: Record<string, string>) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        // Only return existing show for April 15
        if (params?.from_date?.includes("2026-04-15")) {
          return [
            {
              id: 500,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return []; // No shows on April 16
      }
      return {};
    };

    let postCount = 0;
    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 200 + ++postCount }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.skipped).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(result.plans[1].duplicate?.isDuplicate).toBe(false);
  });
});

// -- normalizeDate (timezone conversion) --------------------------------------

describe("normalizeDate", () => {
  test("date-only for Arizona: 8pm MST = 3am UTC next day", () => {
    // Arizona is UTC-7 year-round (no DST)
    expect(normalizeDate("2026-04-15", "AZ")).toBe("2026-04-16T03:00:00Z");
  });

  test("date-only for California (PDT): 8pm PDT = 3am UTC next day", () => {
    // California in April is UTC-7 (PDT)
    expect(normalizeDate("2026-04-15", "CA")).toBe("2026-04-16T03:00:00Z");
  });

  test("date-only for New York (EDT): 8pm EDT = midnight UTC", () => {
    // New York in April is UTC-4 (EDT)
    expect(normalizeDate("2026-04-15", "NY")).toBe("2026-04-16T00:00:00Z");
  });

  test("date-only for Texas (CDT): 8pm CDT = 1am UTC next day", () => {
    // Texas in April is UTC-5 (CDT)
    expect(normalizeDate("2026-04-15", "TX")).toBe("2026-04-16T01:00:00Z");
  });

  test("date+time without timezone for Arizona", () => {
    // 7:30pm Phoenix = 2:30am UTC next day
    expect(normalizeDate("2026-04-15T19:30", "AZ")).toBe("2026-04-16T02:30:00Z");
  });

  test("date+time+seconds without timezone for Arizona", () => {
    expect(normalizeDate("2026-04-15T19:30:00", "AZ")).toBe("2026-04-16T02:30:00Z");
  });

  test("already has timezone suffix (Z): returns as-is", () => {
    expect(normalizeDate("2026-04-15T20:00:00Z", "AZ")).toBe("2026-04-15T20:00:00Z");
  });

  test("already has timezone offset: returns as-is", () => {
    expect(normalizeDate("2026-04-15T20:00:00-07:00", "AZ")).toBe("2026-04-15T20:00:00-07:00");
  });

  test("defaults to Phoenix timezone when no state provided", () => {
    // Same as AZ
    expect(normalizeDate("2026-04-15")).toBe("2026-04-16T03:00:00Z");
  });

  test("California winter (PST, UTC-8): 8pm = 4am UTC next day", () => {
    // January = PST = UTC-8
    expect(normalizeDate("2026-01-15", "CA")).toBe("2026-01-16T04:00:00Z");
  });

  // -- non-US venues (PSY-1873) ---------------------------------------------

  test("venue timezone anchors a Leeds date-only show at 20:00 local", () => {
    // The production defect, exactly: without the venue's zone, "England" runs
    // through the US state map and lands on 20:00 America/Phoenix, which is
    // 03:00Z the NEXT day, rendered as "Sat, Oct 24, 4:00 AM" in Europe/London.
    expect(normalizeDate("2026-10-23", "England")).toBe("2026-10-24T03:00:00Z");
    expect(normalizeDate("2026-10-23", "England", "Europe/London")).toBe(
      "2026-10-23T19:00:00Z",
    );
  });

  test("venue timezone anchors a Berlin date-only show at 20:00 local", () => {
    // CEST in August = UTC+2.
    expect(normalizeDate("2026-08-14", "DE", "Europe/Berlin")).toBe(
      "2026-08-14T18:00:00Z",
    );
  });

  test("venue timezone applies to a stated wall-clock time too", () => {
    expect(normalizeDate("2026-10-23T23:30", "England", "Europe/London")).toBe(
      "2026-10-23T22:30:00Z",
    );
  });

  test("venue timezone east of UTC does not roll the calendar day", () => {
    // A Tokyo evening show is the same calendar day in UTC; the Phoenix
    // fallback would place it a day earlier in local terms.
    expect(normalizeDate("2026-10-23", "", "Asia/Tokyo")).toBe(
      "2026-10-23T11:00:00Z",
    );
  });

  test("a US venue's own timezone agrees with the state map", () => {
    // The fix must not move any US row: both spellings resolve identically.
    expect(normalizeDate("2026-04-15", "AZ", "America/Phoenix")).toBe(
      normalizeDate("2026-04-15", "AZ"),
    );
  });

  test("an unloadable venue timezone falls back to the state map", () => {
    // A bad venues.timezone must not crash the writer or silently become UTC.
    expect(normalizeDate("2026-04-15", "NY", "Mars/Olympus")).toBe(
      "2026-04-16T00:00:00Z",
    );
  });

  test("an empty venue timezone falls back to the state map", () => {
    expect(normalizeDate("2026-04-15", "NY", "")).toBe("2026-04-16T00:00:00Z");
  });
});

// -- venue zone -------------------------------------------------------------

describe("venueZone", () => {
  test("carries the matched venue ROW's state and zone, not the batch's claim", async () => {
    // The wiring under test is resolveVenues copying `state` off the search hit.
    // Without it every existing venue reads as stateless and the times refuse.
    const client = createMockClient({
      get: async () => ({
        venues: [
          { id: 7, name: "Lincoln Hall", slug: "lincoln-hall", city: "Chicago", state: "IL", timezone: "America/Chicago" },
        ],
      }),
    });

    const resolved = await resolveVenues(client, [
      { name: "Lincoln Hall", city: "Chicago", state: "NY" },
    ]);
    expect(resolved[0].matchedState).toBe("IL");
    expect(resolved[0].timezone).toBe("America/Chicago");

    expect(venueZone(resolved, { state: "NY", venues: [{ name: "Lincoln Hall", state: "NY" }] })).toEqual({
      state: "IL",
      timezone: "America/Chicago",
    });
  });

  test("a stored empty state is an answer, and it is carried as one", async () => {
    const client = createMockClient({
      get: async () => ({
        venues: [{ id: 7, name: "Lincoln Hall", slug: "lincoln-hall", city: "Chicago", state: "" }],
      }),
    });

    const resolved = await resolveVenues(client, [
      { name: "Lincoln Hall", city: "Chicago", state: "IL" },
    ]);
    expect(resolved[0].matchedState).toBe("");
    expect(venueZone(resolved, { state: "IL", venues: [{ name: "Lincoln Hall", state: "IL" }] })).toEqual({
      state: "",
      timezone: undefined,
    });
  });

  test("a venue this run will create has no row, so the stated location stands", () => {
    const resolved = [{ name: "Boom Leeds", state: "England", status: "new" as const }];
    expect(venueZone(resolved, { state: "England", venues: [{ name: "Boom Leeds", state: "England" }] })).toEqual({
      state: "England",
    });
  });

  test("the dedup window and the written instant read one zone", () => {
    // These two used to be resolved separately, so a batch claiming a state the
    // matched row disagrees with searched one calendar day and wrote another.
    const plan = planWithTimes({ music_at: "9:00PM" });
    plan.venues[0].timezone = undefined;
    plan.venues[0].matchedState = "AZ";
    plan.input.state = "IL";
    plan.input.venues[0].state = "IL";

    const zone = venueZone(plan.venues, plan.input);
    const written = buildShowPayload(plan).event_date as string;
    const window = showDedupWindow(plan.input.event_date, zone.state, zone.timezone);

    expect(Date.parse(window.fromDate)).toBeLessThanOrEqual(Date.parse(written));
    expect(Date.parse(written)).toBeLessThanOrEqual(Date.parse(window.toDate));
  });

  test("a zoned event_date puts the window on the VENUE's day, not the UTC one", () => {
    // 2026-09-06T01:00:00Z is the evening of September 5 in Chicago, and it is
    // the shape this CLI itself writes. A window sliced off the leading ten
    // characters lands on the 6th and misses the row the writer stores.
    const plan = planWithTimes({ music_at: "8:00 PM" });
    plan.input.event_date = "2026-09-06T01:00:00Z";

    const zone = venueZone(plan.venues, plan.input);
    const written = buildShowPayload(plan).event_date as string;
    const window = showDedupWindow(plan.input.event_date, zone.state, zone.timezone);

    expect(Date.parse(window.fromDate)).toBeLessThanOrEqual(Date.parse(written));
    expect(Date.parse(written)).toBeLessThanOrEqual(Date.parse(window.toDate));
  });

  test("an absent row state falls through to the show's, an empty one does not", () => {
    // Mirrors `venue?.state ?? show.state` in showTimingInput: a stored empty
    // string is the row's answer; only an absent field falls through.
    const absent = [{ id: 7, name: "V", status: "existing" as const }];
    expect(venueZone(absent, { state: "IL", venues: [{ name: "V" }] }).state).toBe("IL");

    const empty = [{ id: 7, name: "V", matchedState: "", status: "existing" as const }];
    expect(venueZone(empty, { state: "IL", venues: [{ name: "V" }] }).state).toBe("");
  });

  test("event_date follows the venue row too, not only the clocks", () => {
    // The row is stateless and unzoned, so every clock on this show resolves
    // through the America/Phoenix default -- including the date-only anchor,
    // which is the zone the page will bucket the day in.
    const plan = planWithTimes({});
    plan.venues[0].timezone = undefined;
    plan.venues[0].matchedState = "";
    plan.input.state = "IL";
    plan.input.venues[0].state = "IL";

    expect(buildShowPayload(plan).event_date).toBe("2026-09-05T03:00:00Z");
  });
});

// -- refusal copy -----------------------------------------------------------

describe("describeShowTimeRefusal", () => {
  const cases: Array<[ShowTimesRefusal, string]> = [
    [{ reason: "no-timezone" }, "no timezone is known"],
    [{ reason: "no-calendar-day", eventDate: "next Friday" }, "next Friday"],
    [{ reason: "unreadable-music", music: "TBD" }, "TBD"],
    [{ reason: "doors-without-music", doors: "7:30PM" }, "no music time"],
    [{ reason: "unreadable-doors", doors: "doors at 7" }, "doors at 7"],
    [
      { reason: "music-before-doors", doors: "11:00 PM", music: "12:00 AM" },
      "is before doors at",
    ],
    [
      { reason: "clock-does-not-exist", clock: "2:30 AM", day: "2026-03-08" },
      "does not exist on 2026-03-08",
    ],
  ];

  for (const [refusal, fragment] of cases) {
    test(`${refusal.reason} names the reason`, () => {
      const line = describeShowTimeRefusal(refusal);
      expect(line).toContain(fragment);
      expect(line.length).toBeGreaterThan(20);
    });
  }

  test("strips control characters out of a source-supplied value", () => {
    // A cursor-movement escape in a scraped value would rewrite lines already
    // printed for an earlier show in the same preview.
    const line = describeShowTimeRefusal({
      reason: "unreadable-music",
      music: "TBD\u001b[2K\u001b[8A rewritten",
    });
    expect(line).not.toContain("\u001b");
    expect(line).toContain("TBD[2K[8A rewritten");
  });

  test("caps a value long enough to scroll the batch out of the terminal", () => {
    const line = describeShowTimeRefusal({
      reason: "unreadable-doors",
      doors: "x".repeat(5000),
    });
    expect(line.length).toBeLessThan(300);
    expect(line).toContain("...");
  });
});

// -- backfilling an existing show -------------------------------------------

describe("timesToBackfill", () => {
  const times = { doorsAt: "2026-09-05T00:30:00Z", musicAt: "2026-09-05T01:30:00Z", refusals: [] };

  test("fills both columns when the stored row has neither", () => {
    expect(timesToBackfill({}, times)).toEqual({
      doorsAt: "2026-09-05T00:30:00Z",
      musicAt: "2026-09-05T01:30:00Z",
    });
  });

  test("fills only the absent column", () => {
    expect(timesToBackfill({ music_at: "2026-09-05T02:00:00Z" }, times)).toEqual({
      doorsAt: "2026-09-05T00:30:00Z",
    });
  });

  test("NEVER overwrites a stored time", () => {
    const stored = { doors_at: "2026-09-05T00:00:00Z", music_at: "2026-09-05T02:00:00Z" };
    expect(timesToBackfill(stored, times)).toBeNull();
  });

  test("treats null and empty string as absent, like the API serves them", () => {
    expect(timesToBackfill({ doors_at: null, music_at: null }, times)).toEqual({
      doorsAt: "2026-09-05T00:30:00Z",
      musicAt: "2026-09-05T01:30:00Z",
    });
  });

  test("writes nothing when the run states no times", () => {
    expect(timesToBackfill({}, { refusals: [] })).toBeNull();
  });

  test("refuses a fill that would invert the stored pair", () => {
    // Stored music 00:00, incoming doors 00:30: the row would end up with music
    // before doors, which validateShowTimeOrder rejects. Do not send it.
    const stored = { music_at: "2026-09-05T00:00:00Z" };
    expect(timesToBackfill(stored, { doorsAt: "2026-09-05T00:30:00Z", refusals: [] })).toBeNull();
  });
});

describe("backfillShowTimes", () => {
  test("sends only the absent column and leaves the rest of the show alone", async () => {
    let putPath = "";
    let putBody: unknown = null;
    const client = createMockClient({
      get: async () => ({ id: 9, doors_at: null, music_at: "2026-09-05T02:00:00Z" }),
    });
    (client as unknown as Record<string, unknown>).put = async (path: string, body: unknown) => {
      putPath = path;
      putBody = body;
      return {};
    };

    const filled = await backfillShowTimes(client, 9, {
      doorsAt: "2026-09-05T00:30:00Z",
      musicAt: "2026-09-05T01:30:00Z",
      refusals: [],
    });

    expect(filled).toEqual({ doorsAt: "2026-09-05T00:30:00Z" });
    expect(putPath).toBe("/shows/9");
    // Omitting music_at is what leaves the stored one alone: the field is
    // tri-state on the API, and sending null would CLEAR it.
    expect(putBody).toEqual({ doors_at: "2026-09-05T00:30:00Z" });
  });

  test("issues no request when there is nothing to fill", async () => {
    let putCalls = 0;
    const client = createMockClient({
      get: async () => ({ id: 9, doors_at: "2026-09-05T00:00:00Z", music_at: "2026-09-05T02:00:00Z" }),
    });
    (client as unknown as Record<string, unknown>).put = async () => {
      putCalls++;
      return {};
    };

    const filled = await backfillShowTimes(client, 9, {
      doorsAt: "2026-09-05T00:30:00Z",
      musicAt: "2026-09-05T01:30:00Z",
      refusals: [],
    });
    expect(filled).toBeNull();
    expect(putCalls).toBe(0);
  });
});
