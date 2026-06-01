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
