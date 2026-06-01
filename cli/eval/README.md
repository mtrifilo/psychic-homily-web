# /ingest Extraction Evals

Regression net for the **extraction** step of the `/ingest` skill — the part
where a vision model reads a flyer / festival lineup / playlist screenshot and
produces batch JSON. The `ph` CLI's *submission* step (validate, dedup, resolve,
POST) is already covered by `cli/test/`; this harness covers the previously
untested vision-to-JSON step.

These evals are **not wired into CI** (PSY-935 decision). Run them manually when
you change the extraction prompt, the skill's rules, or the model.

## What's here

| File                       | Role                                                                 |
| -------------------------- | -------------------------------------------------------------------- |
| `extraction-prompt.md`     | The extraction prompt — **single source of truth**, shared with `.claude/skills/ingest/SKILL.md`. Edit extraction rules HERE. |
| `batch-schema.json`        | JSON Schema for the batch output (mirrors what `ph batch` consumes). |
| `prompt.ts`                | promptfoo prompt function: injects `extraction-prompt.md` text + the fixture image into an Anthropic multimodal message. |
| `scoring.ts`               | Pure, unit-tested scoring logic (artists found/missed/hallucinated, venue, festival fields, billing-tier agreement). |
| `assert.ts`                | promptfoo custom assertion adapting `scoring.ts` + schema validation into a GradingResult. |
| `promptfooconfig.yaml`     | The eval config (prompt, provider, fixtures, assertion).             |
| `fixtures/`                | Golden image + expected-JSON fixtures (`fixtures/README.md`).        |

The scoring logic has direct `bun test` coverage in `cli/test/eval-scoring.test.ts`,
so the regression net is partly verified without spending any API budget.

## Running the evals

### 1. Provide an Anthropic API key

The harness calls the Anthropic API directly. Export your key (never commit it):

```bash
export ANTHROPIC_API_KEY="sk-ant-..."   # use your own key; do not commit
```

> The repo's documented env var is `ANTHROPIC_API_KEY` (same name the backend
> discovery pipeline uses; see `frontend/.env.example`). Use a key with vision
> access. The eval reads it from the environment only — it is never written to
> a file or printed.

### 2. Run

```bash
cd cli
bun install        # first time (installs promptfoo + ajv)
bun run eval       # runs promptfoo against every fixture
```

`bun run eval` prints a per-fixture table. The assertion's `reason` field carries
the full per-entity breakdown (artists found/missed/hallucinated, venue, festival
fields, billing-tier agreement, overall score). To browse results in the
promptfoo UI:

```bash
bun run eval:view
```

### 3. Read the scores

The assertion returns `namedScores` per fixture:

- `artist_recall` — fraction of golden artists the model produced
- `artists_missed` / `artists_hallucinated` — counts (in the component breakdown)
- `venue_recall` — fraction of golden venues produced
- `festival_<field>` — per-field correctness (name, slug, year, start_date, end_date)
- `billing_tier_agreement` — fraction of lineup artists whose billing tier matches
- `schema_valid` — 1 if the output conforms to `batch-schema.json`
- `overall` — weighted summary (artists 55%, festival fields 20%, billing 15%, venue 10%)

**No hard pass/fail accuracy gate is set.** The eval reports scores; it does not
fail a build on low accuracy (PSY-935 — threshold-setting is a follow-up user
decision now that baselines exist). The one fatal condition is schema-invalid
output (`pass: false`), which means the extraction produced something `ph batch`
could not consume.

## Harness decision: Promptfoo (spike resolved)

**Promptfoo is wired up.** The PSY-935 spike asked whether Promptfoo supports
image inputs to Anthropic models. It does: the `anthropic:messages:` provider
accepts base64 image content blocks, and promptfoo auto-base64-encodes a
`file://...png` test var. We verified the full request reaches the Anthropic API
with the correct multimodal payload (text instructions + image block). The
prompt function (`prompt.ts`) builds the message from `extraction-prompt.md` so
the prompt text never duplicates / drifts from the source of truth.

Two non-obvious gotchas we hit:

- **Prompt-function reference:** use `file://prompt.ts` (NOT `file://prompt.ts:default`).
  promptfoo resolves the module's default export itself; appending `:default`
  makes it look for a `.default` property *on* the already-unwrapped function and
  fails with `promptFunction is not a function`.
- **Scoring lives in `scoring.ts`, not the assertion**, so it is unit-testable
  without the API and reusable by a plain-`bun test` fallback harness if
  promptfoo is ever dropped.

If a future promptfoo release regresses Anthropic image support, the fallback is
a plain `bun test` harness that calls the Anthropic SDK directly with the image
and feeds the response to `scoring.ts` — the scoring half already works that way.

## Adding a fixture

See [`fixtures/README.md`](./fixtures/README.md).
