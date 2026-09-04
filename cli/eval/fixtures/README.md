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
    └── expected.json   # the human-verified golden batch JSON
```

The image and golden JSON are versioned in the repo so the eval is reproducible
from a fresh clone. (Images are large — the user accepted the weight in exchange
for reproducibility; see PSY-935.)

## Current fixtures

| Slug                | Source                       | Shape                                  | Verified |
| ------------------- | ---------------------------- | -------------------------------------- | -------- |
| `riot-fest-2026`    | Riot Fest 2026 poster        | 1 venue + 102 artists + 1 festival (lineup with billing tiers) | 2026-05-31 Stage ingest, 100% link rate |
| `lh-st-lincoln-hall-2026-09` | `https://lh-st.com/` events page, captured 2026-09-04 | 1 venue + 2 artists + 1 show, **labelled** `Doors 7:30PM` / `Show 8:30PM` | golden read off the capture; `2026` from the site's own show slug `/shows/09-04-2026-wolves-of-glendale/`, which the image does not print |
| `empty-bottle-2026-09` | `https://www.emptybottle.com/` events page, captured 2026-09-04 | 1 venue + 5 artists + 2 shows, each printing ONE **unlabelled** time | golden read off the capture; years from the page's own listing order, which the image does not print |
| `sinkhole-2026-09` | `https://sinkholerecords.com/events/` events page, captured 2026-09-04 | 1 venue + 3 artists + 1 show, `Doors: 7 pm // Show: 8 pm` labelled but **printed with no minutes** | golden read off the capture; the poster prints the date, year, city and venue |

### The door-time set

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
source, and that is not a contradiction. There the transform reads the DOM, whose
`.start-time` class publishes the event's start; the owner's 2026-09-04 decision
is that a published start time IS `music_at`. This fixture is a CAPTURE of the
rendered card, where no field name and no label are on screen and all a reader
has is a bare clock. The two paths see genuinely different sources, so they
answer differently on purpose: the transform reads a named field, the vision step
reads pixels.

Both goldens carry `city` / `state` that the images do NOT print. The batch
schema requires them on a show and a venue, and a model that knows Lincoln Hall
and Empty Bottle supplies Chicago, IL; a fixture that omitted them would fail
the schema gate for a reason unrelated to what it is testing. They are not
scored.

- **Minutes-less** - The Sinkhole labels both times and prints them as bare
  hours. The golden states `"7:00 pm"` / `"8:00 pm"`, because the shared parser
  refuses a meridiem clock with no `:MM` and adding `:00` states nothing the
  source did not. A model that copies `7 pm` through scores zero on that show:
  the clock is unreadable, and an unreadable stated time is not a stored one.

All three are live captures rather than posters because the `ph batch` path
ingests venue calendars; the registry of those calendars is in
`.claude/skills/ingest/references/venue-events.md`.

**The prompt's worked examples are deliberately NOT these cards.** Rule 6 in
`../extraction-prompt.md` illustrates the rule with shapes no fixture uses
(`DOORS 6:00 PM · MUSIC 6:45 PM`, `Doors: 5 pm // Show: 6 pm`,
`TUE FEB 17 · 11:15PM`). An earlier revision used the LH-ST and Empty Bottle card
text verbatim, which measured whether the model could copy its own instructions
rather than read the image. Keep new examples off the corpus.

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

The Riot Fest poster is a festival lineup. To exercise the other extraction
shapes the `/ingest` skill handles, add fixtures for:

- a **single-show flyer** (one venue, one date, headliner + openers)
- a **multi-show tour post** (several dates, one lineup, @handles in the caption)
- a **WFMU radio playlist** screenshot (artists → releases → labels, years)
