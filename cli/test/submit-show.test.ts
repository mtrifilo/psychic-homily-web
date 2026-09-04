import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseShowInput,
  resolveArtists,
  resolveVenues,
  buildShowPayload,
  planShowTimes,
  normalizeDate,
  showPriceLine,
  submitShows,
  type ShowPlan,
} from "../src/commands/submit-show";
import { APIClient } from "../src/lib/api";
import { checkShowDuplicate } from "../src/lib/duplicates";
import { validateShow } from "../src/lib/schemas";

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

  test("a venue with no resolvable zone writes neither column and says why", () => {
    const plan = planWithTimes({ doors_at: "7:30PM", music_at: "8:30PM" });
    plan.input.state = "England";
    plan.input.venues[0].state = "England";
    plan.venues[0].state = "England";
    plan.venues[0].timezone = undefined;

    const payload = buildShowPayload(plan);
    expect("doors_at" in payload).toBe(false);
    expect("music_at" in payload).toBe(false);
    expect(planShowTimes(plan).notes[0]).toContain("no timezone is known");
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
