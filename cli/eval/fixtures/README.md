# Ingest Extraction Eval Fixtures

Each fixture is a golden, human-verified example of the `/ingest` extraction step:
an input image plus the exact batch JSON it should produce. The eval harness
(`cli/eval/`) runs the extraction prompt against the image and scores the model's
output against the golden JSON.

## Layout

```
fixtures/
└── <fixture-slug>/
    ├── poster.png      # the input image (flyer / lineup / playlist screenshot)
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

### The two show-flyer fixtures

They are a matched positive/negative pair on the same two rules: `door_price`
and `artists[].set_type` are emitted ONLY when the source states them.

- `split-price-stated-roles` states both. A model that drops the door price, or
  flattens a stated role, fails it.
- `single-price-unstated-roles` states neither, and lists its four acts at one
  type size. A model that copies `price` into `door_price`, or reads a headliner
  off the top of the list, fails it.

They are **synthetic**: the flyers are rendered from HTML in this repo's history
rather than captured from a promoter, and the golden JSON is verified by
construction because the poster text was authored alongside it. That is the
trade for testing a rule no production capture in the corpus exercises; every
other fixture should still come from a real source, human-verified against a
dry run. The venues are real Phoenix rooms and the acts are invented.

Scored by `show_price_agreement` and `bill_role_agreement`, which compare
ABSENCE as well as value — an unstated door price must come back unstated. Both
grade at 1.0 rather than the 0.8 the recall metrics use, because a single
spurious value is the whole failure. Neither feeds `overall`, so the Riot Fest
baseline stays comparable.

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
