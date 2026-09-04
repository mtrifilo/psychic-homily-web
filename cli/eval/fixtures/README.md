# Ingest Extraction Eval Fixtures

Each fixture is a golden, human-verified example of the `/ingest` extraction step:
an input image plus the exact batch JSON it should produce. The eval harness
(`cli/eval/`) runs the extraction prompt against the image and scores the model's
output against the golden JSON.

## Layout

```
fixtures/
└── <fixture-slug>/
    ├── poster.png      # the input image (flyer / lineup / playlist screenshot
    │                   # / venue-listing capture; name it for what it is:
    │                   # promptfooconfig.yaml points at the file by name)
    ├── poster.html     # (synthetic fixtures only) the source the PNG renders from
    └── expected.json   # the human-verified golden batch JSON
```

The image and golden JSON are versioned in the repo so the eval is reproducible
from a fresh clone. (Images are large — the user accepted the weight in exchange
for reproducibility; see PSY-935.)

## Current fixtures

| Slug                | Source                       | Shape                                  | Verified |
| ------------------- | ---------------------------- | -------------------------------------- | -------- |
| `riot-fest-2026`    | Riot Fest 2026 poster        | 1 venue + 102 artists + 1 festival (lineup with billing tiers) | 2026-05-31 Stage ingest, 100% link rate |
| `split-price-stated-roles`   | **Synthetic** show flyer | 1 venue + 3 artists + 1 show; states `$20 ADV / $25 DOOR` and a role for every act | by construction (see below) |
| `single-price-unstated-roles` | **Synthetic** show flyer | 1 venue + 4 artists + 1 show; states one price and no roles | by construction (see below) |
| `lh-st-lincoln-hall-2026-09` | `https://lh-st.com/` events page, captured 2026-09-04 | 1 venue + 2 artists + 1 show, **labelled** `Doors 7:30PM` / `Show 8:30PM` | golden read off the capture; `2026` from the site's own show slug `/shows/09-04-2026-wolves-of-glendale/`, which the image does not print |
| `empty-bottle-2026-09` | `https://www.emptybottle.com/` events page, captured 2026-09-04 | 1 venue + 5 artists + 2 shows, each printing ONE **unlabelled** time | golden read off the capture; years from the page's own listing order, which the image does not print |

### The two show-flyer fixtures

They are a matched positive/negative pair on the same two rules: `door_price`
and `artists[].set_type` are emitted ONLY when the source states them.

- `split-price-stated-roles` states both. A model that drops the door price, or
  flattens a stated role, fails it.
- `single-price-unstated-roles` states neither, and lists its four acts at one
  type size. A model that copies `price` into `door_price`, or reads a headliner
  off the top of the list, fails it.

They are **synthetic**: each `poster.png` renders from the `poster.html` beside
it rather than being captured from a promoter, and the golden JSON is verified
by construction because the poster text was authored alongside it. That is the
trade for testing a rule no production capture in the corpus exercises; every
other fixture should still come from a real source, human-verified against a
dry run. The venues are real Phoenix rooms and the acts are invented.

Each poster carries only facts the extraction rules cover, so the golden is the
whole of what a faithful extraction produces from it.

To amend one, edit `poster.html`, re-render it at the size its own `body` sets,
and update `expected.json` to match. Nothing checks that the committed PNG was
rendered from the committed HTML, so re-render in the same change:

```bash
cd frontend && ./node_modules/.bin/playwright screenshot --viewport-size=800,1100 \
  "file://$(git rev-parse --show-toplevel)/cli/eval/fixtures/<slug>/poster.html" \
  "$(git rev-parse --show-toplevel)/cli/eval/fixtures/<slug>/poster.png"
```

They are scored by `show_price_agreement` and `bill_role_agreement` (see
`cli/eval/assert.ts` for how those are graded).

### The door-time pair

The two venue-listing fixtures are a matched positive/negative case for
`doors_at` / `music_at`, and they are only meaningful together:

- **Positive**: Lincoln Hall labels both times, so both belong on the show.
- **Negative**: Empty Bottle prints a bare `10:00PM` with no label. The golden
  states NEITHER field: nothing on the rendered page says whether that clock is
  doors or the first set, and `ph batch` refuses a time the source did not name.
  A model that files it as `music_at` loses that listing's schedule, so labelling
  one of the two cards scores 0.5 and labelling both scores 0; either way the
  value it invented is named in the assertion's reason.

Empty Bottle appears in the registry's own per-source table as a `music_at`
source, which is not a contradiction: there the transform reads the DOM, whose
`.start-time` class names the role. This fixture is a CAPTURE of the rendered
card, where that name is not on screen. The two paths see different sources and
so answer differently, which means one Empty Bottle show gets a `music_at` or
not depending on which path ingested it. That split is a live inconsistency, not
a settled design; it is recorded here so the decision is made deliberately.

Both goldens carry `city` / `state` that the images do NOT print. The batch
schema requires them on a show and a venue, and a model that knows Lincoln Hall
and Empty Bottle supplies Chicago, IL; a fixture that omitted them would fail
the schema gate for a reason unrelated to what it is testing. They are not
scored.

Both are live captures rather than posters because the `ph batch` path ingests
venue calendars; the registry of those calendars is in
`.claude/skills/ingest/references/venue-events.md`.

## Adding a new fixture

1. **Capture the input.** Save the source image as
   `fixtures/<slug>/poster.png` (or `.jpg`). Use a descriptive slug
   (`wfmu-playlist-2026-06`, `valley-bar-tour-flyer`, ...).
2. **Produce + verify the golden JSON.** Run the real `/ingest` flow against the
   image, then human-verify the dry-run against Stage (names, dates, venue,
   billing tiers, link rate) exactly as the Riot Fest fixture was verified. Save
   the verified batch JSON as `fixtures/<slug>/expected.json`. It MUST conform to
   `cli/eval/batch-schema.json`.
3. **Register the test case** in `cli/eval/promptfooconfig.yaml` — add a new entry
   under `tests:` pointing at the new image + expected JSON:
   ```yaml
     - description: "<what this fixture covers>"
       vars:
         image: file://fixtures/<slug>/poster.png
         media_type: image/png        # or image/jpeg
         expected_json: file://fixtures/<slug>/expected.json
   ```
4. **Re-run** `cd cli && bun run eval` and record the baseline in the PR.

## Target fixture coverage (future)

The two show-flyer fixtures are synthetic and cover one rule each way. Still
missing, all from real sources:

- a **captured single-show flyer** (one venue, one date, real promoter artwork)
- a **multi-show tour post** (several dates, one lineup, @handles in the caption)
- a **WFMU radio playlist** screenshot (artists → releases → labels, years)
