import { describe, test, expect, spyOn, afterEach } from "bun:test";
import {
  artistNameFlags,
  catalogueArtistLookup,
  equalsSubtitle,
  eventWording,
  joinedParts,
  reportArtistNameFlags,
  roomOrCity,
  sourceSubtitles,
  wordCount,
  type ArtistNameContext,
  type ArtistNamePlan,
} from "../src/lib/artistNameFlags";
import { APIClient } from "../src/lib/api";

const launchpad: ArtistNameContext = { rooms: ["Launchpad"], cities: ["Albuquerque"], subtitles: [] };
const noneKnown = async (_part: string) => false;
const rules = async (name: string, context = launchpad, isArtist = noneKnown) =>
  (await artistNameFlags(name, context, isArtist)).map((f) => f.rule);

describe("event-like wording", () => {
  test.each([
    ["LIVE AT LAUNCHPAD", "live at"],
    ["Jacob Eary Quartet Plays the Music of Ornette Coleman", "plays"],
    ["Puppeteers For Fears presents Robopocalypse", "presents"],
    ["Michael Shannon & Jason Narducy and Friends play R.E.M.'s Document", "and friends"],
    ["ted.fm & friends", "& friends"],
    ["Lone Stranger &Friends", "& friends"],
    ["Hyperpop Night", "night"],
    ["Grand Royale - Beastie Boys Tribute", "tribute"],
    ["Remembering Ray Gutierrez ~ Memorial Concert", " ~ "],
  ])("%s", (name: string, want: string) => {
    expect(eventWording(name)).toBe(want);
  });

  test("matches whole words only", () => {
    expect(eventWording("Nightwish")).toBeNull();
    expect(eventWording("Tributary")).toBeNull();
    expect(eventWording("Presentation")).toBeNull();
    expect(eventWording("Friendship Commanders")).toBeNull();
    expect(eventWording("Tilde~Joined")).toBeNull();
  });

  // The hand check of the audit's sample found these four real acts among the
  // names the wording rule flags. They stay flaggable, because the rule is
  // flag-only, and are pinned here so its false-positive rate is on record:
  // 4 of 50 in the sample.
  test.each(["Ganja White Night", "Late Night Radio", "Our Last Night", "Sunshine & The Night Walkers"])(
    "flags the real act %s (a known false positive)",
    async (name: string) => {
      expect(eventWording(name)).toBe("night");
      expect(await rules(name)).toEqual(["event-wording"]);
    },
  );
});

describe("the billed room or its city", () => {
  test("finds the room's name as whole words, with a leading 'the' dropped", () => {
    expect(roomOrCity("LIVE AT LAUNCHPAD", ["Launchpad"], [])).toBe(`the room's name "Launchpad"`);
    expect(roomOrCity("Fabio Frizzi at THE TEMPLE", ["The Temple"], [])).toBe(`the room's name "The Temple"`);
    expect(roomOrCity("Launchpads", ["Launchpad"], [])).toBeNull();
  });

  test("finds the city as whole words", () => {
    expect(roomOrCity("Show Up Chicago", ["Thalia Hall"], ["Chicago"])).toBe(`the city "Chicago"`);
    expect(roomOrCity("Junior Mesa Live in Los Angeles NIGHT 2", ["The Echo"], ["Los Angeles"])).toBe(`the city "Los Angeles"`);
  });

  test("ignores rooms and cities shorter than four characters", () => {
    expect(roomOrCity("The Den Band", ["The Den"], ["Ada"])).toBeNull();
    expect(roomOrCity("Ada Band", [], ["Ada"])).toBeNull();
  });
});

describe("five or more words", () => {
  test("counts words after folding punctuation", () => {
    expect(wordCount("Dead on a Sunday and Above Snakes")).toBe(7);
    expect(wordCount("Mannie Fresh & DJ Soul Sister")).toBe(5);
    expect(wordCount("Harry Katz & The Pistachios")).toBe(4);
  });

  test("flags five words and not four", async () => {
    expect(await rules("Forever Came Calling and Fox", { rooms: [], cities: [], subtitles: [] })).toEqual(["five-words"]);
    expect(await rules("Charles Latham Borrowed Band", { rooms: [], cities: [], subtitles: [] })).toEqual([]);
  });
});

describe("names joined from existing artists", () => {
  test("splits on and, &, +, w/ and commas", () => {
    expect(joinedParts("Darby O'Trill and Mikey Rotten")).toEqual(["Darby O'Trill", "Mikey Rotten"]);
    expect(joinedParts("Tara Zorthian, Ranch House Motel, Stine")).toEqual(["Tara Zorthian", "Ranch House Motel", "Stine"]);
    expect(joinedParts("Snail Mail & Soccer Mommy")).toEqual(["Snail Mail", "Soccer Mommy"]);
    expect(joinedParts("Wax + DJ Hoppa")).toEqual(["Wax", "DJ Hoppa"]);
    expect(joinedParts("Lowheaven w/ Infinite Loop")).toEqual(["Lowheaven", "Infinite Loop"]);
    expect(joinedParts("Slowdive")).toBeNull();
  });

  test("flags only when every part is an artist", async () => {
    const known = new Set(["wax", "dj hoppa"]);
    const isArtist = async (p: string) => known.has(p.toLowerCase());
    const none: ArtistNameContext = { rooms: [], cities: [], subtitles: [] };
    expect(await rules("Wax and DJ Hoppa", none, isArtist)).toEqual(["joins-artists"]);
    expect(await rules("Wax and Someone", none, isArtist)).toEqual([]);
  });

  test("a lookup that fails is flagged as unchecked, not passed", async () => {
    const failing = async () => {
      throw new Error("search unavailable");
    };
    const flags = await artistNameFlags("Amyl and The Sniffers", { rooms: [], cities: [], subtitles: [] }, failing);
    expect(flags).toEqual([
      {
        rule: "joins-artists",
        detail: `joins 2 names ("Amyl", "The Sniffers"); could not check whether each is an artist: search unavailable`,
      },
    ]);
  });
});

describe("a subtitle line of the source", () => {
  test("matches a subtitle once folded", async () => {
    expect(equalsSubtitle("LIVE AT LAUNCHPAD", ["Live at Launchpad!"])).toBe(true);
    expect(equalsSubtitle("Ouija Macc", ["Live at Launchpad"])).toBe(false);
    expect(await rules("Ouija Macc", { rooms: [], cities: [], subtitles: ["Ouija Macc"] })).toEqual(["source-subtitle"]);
  });

  test("reads one subtitle or a list from the show", () => {
    expect(sourceSubtitles({ source_subtitles: "LIVE AT LAUNCHPAD" })).toEqual(["LIVE AT LAUNCHPAD"]);
    expect(sourceSubtitles({ source_subtitles: ["a", 3, "b"] })).toEqual(["a", "b"]);
    expect(sourceSubtitles({})).toEqual([]);
  });
});

test("the ticket's example trips three rules at once", async () => {
  const context = { ...launchpad, subtitles: ["LIVE AT LAUNCHPAD"] };
  expect(await rules("LIVE AT LAUNCHPAD", context)).toEqual(["event-wording", "room-or-city", "source-subtitle"]);
});

test("a plain act name trips nothing", async () => {
  expect(await rules("Ouija Macc")).toEqual([]);
});

describe("reportArtistNameFlags", () => {
  const plan = (over: Partial<ArtistNamePlan> = {}): ArtistNamePlan => ({
    input: { event_date: "2026-11-21", city: "Albuquerque" },
    artists: [
      { id: 7, name: "Ouija Macc", status: "existing" },
      { name: "LIVE AT LAUNCHPAD", status: "new" },
    ],
    venues: [{ name: "Launchpad", city: "Albuquerque" }],
    valid: true,
    ...over,
  });
  let written = "";
  const stderr = () => {
    written = "";
    return spyOn(process.stderr, "write").mockImplementation((chunk: string | Uint8Array) => {
      written += String(chunk);
      return true;
    });
  };
  afterEach(() => {
    (process.stderr.write as unknown as { mockRestore?: () => void }).mockRestore?.();
  });

  test("prints one flag line per flagged name and nothing for the rest", async () => {
    const spy = stderr();
    await reportArtistNameFlags(new APIClient({ url: "http://localhost:1", token: "t" } as never), [plan()], noneKnown);
    spy.mockRestore();
    expect(written).toContain("flag only: nothing is refused or changed");
    expect(written).toContain(`[1/1] "LIVE AT LAUNCHPAD" (new artist) on 2026-11-21 at Launchpad: event-like wording ("live at"); contains the room's name "Launchpad"`);
    expect(written).not.toContain("Ouija Macc");
  });

  test("skips invalid and duplicate shows, and prints nothing when no name trips a rule", async () => {
    const spy = stderr();
    await reportArtistNameFlags(new APIClient({ url: "http://localhost:1", token: "t" } as never), [
      plan({ valid: false }),
      plan({ duplicate: { isDuplicate: true } }),
      plan({ artists: [{ name: "Ouija Macc", status: "new" }] }),
    ], noneKnown);
    spy.mockRestore();
    expect(written).toBe("");
  });

  test("prints names with control and format characters replaced", async () => {
    const spy = stderr();
    const esc = String.fromCharCode(27);
    const rlo = String.fromCharCode(0x202e);
    await reportArtistNameFlags(new APIClient({ url: "http://localhost:1", token: "t" } as never), [
      plan({ artists: [{ name: `Jazz Night${esc}[2J${rlo}`, status: "new" }] }),
    ], noneKnown);
    spy.mockRestore();
    expect(written).toContain(`"Jazz Night?[2J?" (new artist)`);
    expect(written).not.toContain(esc + "[2J");
  });

  test("names an existing artist by its id", async () => {
    const spy = stderr();
    await reportArtistNameFlags(new APIClient({ url: "http://localhost:1", token: "t" } as never), [
      plan({ artists: [{ id: 99, name: "Ganja White Night", status: "existing" }] }),
    ], noneKnown);
    spy.mockRestore();
    expect(written).toContain(`"Ganja White Night" (existing artist #99)`);
  });
});

describe("catalogueArtistLookup", () => {
  test("counts an exact match ignoring case and accents, and searches each part once", async () => {
    const client = new APIClient({ url: "http://localhost:1", token: "t" } as never);
    const get = spyOn(client, "get").mockImplementation((async (_path: string, params?: Record<string, string>) => {
      if (/beyonce/i.test(params?.q ?? "")) return { artists: [{ id: 1, name: "Beyoncé", slug: "beyonce" }] };
      return { artists: [{ id: 2, name: "Darby O'Trill Band", slug: "darby-otrill-band" }] };
    }) as never);
    const isArtist = catalogueArtistLookup(client);

    expect(await isArtist("Beyonce")).toBe(true);
    expect(await isArtist("beyonce")).toBe(true);
    expect(await isArtist("Darby O'Trill")).toBe(false);
    expect(get).toHaveBeenCalledTimes(2);
    get.mockRestore();
  });
});
