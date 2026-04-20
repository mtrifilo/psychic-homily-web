# E2E Testing — Strategy and Operating Policy

> Closes PSY-436. Operational companion to [`testing-layers.md`](testing-layers.md) (the "which layer" decision) and [`e2e-performance-baseline.md`](../learnings/e2e-performance-baseline.md) (the PSY-417 profiling baseline). This doc answers "how do we keep E2E fast and reliable as it grows?" — the actual layering decisions live in `testing-layers.md`.

## TL;DR

- **PR CI: `@smoke` only** (<3 min). Full suite post-merge, sharded 4-way.
- **Per-worker user model** — every worker owns its own seeded user; mutating tests never share state.
- **Automatic per-worker cleanup** — admin reset endpoint wipes the worker's mutable rows on teardown.
- **Time budget**: p95 per-test < 15s, full suite < 5 min pre-shard, smoke suite < 3 min.
- **Flake policy**: 3 flakes in 2 weeks → fix, quarantine, or delete. No accumulating flakes.
- **Before adding a new E2E**, ask: "Can mocked backend + component test catch this?" If yes, it's not an E2E.

## Infrastructure (as of 2026-04-19)

The suite's reliability and speed rests on layers that landed across April 2026. Treat each as a guardrail — when you see a test pattern that fights one of these, the pattern is probably wrong.

| Layer | Ticket | What it gives you |
|---|---|---|
| Reserved seeded rows | [PSY-430](https://linear.app/psychic-homily/issue/PSY-430) | Mutating tests pin to known-slug rows; no more cross-test slug collisions. |
| Per-worker seeded users | [PSY-431](https://linear.app/psychic-homily/issue/PSY-431) | Worker N uses `e2e-user-N@test.local` with its own auth state. Parallel mutating tests don't race on shared user state. |
| Disable background services | [PSY-433](https://linear.app/psychic-homily/issue/PSY-433) | `DISABLE_*` flags in global-setup silence schedulers, enrichment workers, and reminders during E2E — deterministic DB + no log spam. |
| Worker-scoped cleanup endpoint | [PSY-432](https://linear.app/psychic-homily/issue/PSY-432) | `POST /admin/test-fixtures/reset` wipes a worker user's mutable rows on Playwright worker teardown. Auto-fires even after a crash. |
| Smoke / full split | [PSY-446](https://linear.app/psychic-homily/issue/PSY-446) | `@smoke` tests gate every PR; full sharded suite runs post-merge. |
| 4-way sharding | [PSY-418](https://linear.app/psychic-homily/issue/PSY-418) | Full suite split across 4 GitHub Actions runners; blob reports merged for the final artifact. |
| Profiling + baseline | [PSY-417](https://linear.app/psychic-homily/issue/PSY-417) | [`e2e-performance-baseline.md`](../learnings/e2e-performance-baseline.md) captures per-test and setup costs; update it when the suite grows significantly. |

## Time budget

| Target | Budget |
|---|---|
| p95 per-test wall clock | < 15 s |
| Full suite local wall clock (pre-shard) | < 5 min |
| Full suite CI wall clock (4 shards) | < 3 min per shard |
| `@smoke` suite (PR CI) | < 3 min total |

If a test blows through the p95 per-test budget, it's usually one of: excessive `waitForLoadState('networkidle')` (replace with targeted `waitForResponse`), `pressSequentially` where `fill` would do, or multi-page flows that should be multiple tests. PSY-417's baseline is the reference for "what normal looks like."

## Flake policy

E2E flakes are a tax on every contributor. The rule:

**A test that flakes 3+ times within 2 weeks must be fixed, quarantined, or deleted before the next feature PR touches that area.**

The three outcomes:

- **Fix** — preferred. Surface the real race condition; don't paper over it with timeouts. Recent examples: [PSY-435](https://linear.app/psychic-homily/issue/PSY-435) (role-mismatch selector caused apparent "context closed" errors), [PSY-464](https://linear.app/psychic-homily/issue/PSY-464) (frontend/backend schema mismatch made a UI invariant impossible).
- **Quarantine** — `test.fixme()` with a Linear ticket in the annotation. Acceptable for up to one sprint while the fix is in flight; longer than that, the test should be deleted.
- **Delete** — if the test's value doesn't justify the investigation cost, or if the thing it's asserting is covered by a cheaper layer, delete it. A missing test is more honest than a flaky one.

**Never**:
- Bump a timeout without understanding why the test is slow.
- Add `{ retries: N }` to a single test to mask flake.
- Add `waitForTimeout(N)` for "stability" — you're papering over a real race.

## Per-worker user model

Worker `i` uses the seeded user `e2e-user-${i}@test.local` (or `e2e-user@test.local` for worker 0 — legacy). The auth-state file is `e2e/.auth/user-${i}.json`. Workers are bounded by `USER_COUNT` in `global-setup.ts` — currently 5. Retry workers with index ≥ USER_COUNT modulo back into the pool (safe because the original worker is done by then).

**Consequence**: mutating tests MUST use `authenticatedPage`, not the default `page`. The per-worker fixture in `e2e/fixtures/auth.ts` provides this.

**Consequence**: if you add a 6th seeded user, bump `USER_COUNT` in `global-setup.ts`, `setup-db.sh`, and the worker cap in `playwright.config.ts` all at once.

## Cleanup model

Every worker has a `workerCleanup` fixture (auto-fires, worker-scoped) that:

1. Looks up the worker user's numeric ID via `GET /auth/profile` on the first fixture call.
2. Calls `POST /admin/test-fixtures/reset` on worker teardown, wiping:
   - `user_bookmarks` (saves + favorites + follows + going/interested)
   - `collection_items`, `collection_subscribers`, `collections`
   - `pending_shows` (virtual scope — preserves approved seed data)

The teardown runs even after a test crash, so mid-test failures don't leak into later runs.

Out-of-scope for this endpoint (tests handle these via direct DELETE calls, which is working and should stay): `comments`, `comment_votes`, `field_notes`. If you add a new mutating flow whose table isn't covered, extend the allowlist in `backend/internal/api/handlers/test_fixtures.go` OR add it to the skip map in `test_fixtures_allowlist_test.go` with a justifying comment — the contract test will fail CI on drift.

## When to add a new E2E test

Before adding, check [`testing-layers.md`](testing-layers.md)'s decision tree. Short version:

- **Real browser + real backend required** → E2E is the right layer.
- **Only UI behavior under known API shapes** → component test (Vitest + RTL + mocked fetch).
- **Only DB + service behavior** → Go integration test (testcontainers).
- **Pure functions** → unit test.

Additional rules specific to the E2E suite:

1. **One smoke per flow.** If a flow already has a `@smoke` test, don't tag a second test that exercises the same path. Smoke tests gate every PR — we want breadth, not redundancy.
2. **Mutating tests must reset what they touch.** The worker cleanup covers the allowlisted scopes; anything else (comments, reports, field notes) needs in-test cleanup.
3. **Tests must not share mutable rows.** Use the worker-scoped user or reserved seed rows. Never assume two tests on the same entity can interleave.
4. **Prefer `waitForResponse` over `waitForLoadState('networkidle')`.** The latter waits for *any* network silence and burns time; the former is targeted.
5. **Prefer `fill('...')` over `pressSequentially`.** Debounced searches fire exactly one query either way; atomic fill creates less render churn while we wait for the result.
6. **Match computed ARIA roles.** `<button role="option">` has computed role `option`, not `button`. Playwright's `getByRole` uses the computed role.

## Profiling a slow test locally

```bash
# From /frontend
bunx playwright test e2e/pages/<file>.spec.ts --grep "<name>" \
  --workers=1 --reporter=list --trace=on

# Then open the trace viewer for any failed test
bunx playwright show-trace test-results/*/trace.zip
```

The trace shows per-action timing, network activity, and the DOM at each step. When a test is slow but passes, the trace viewer's timeline is the fastest way to spot the dominant phase (usually a `waitForLoadState('networkidle')` or a pre-debounce `pressSequentially`).

For structural comparisons against the PSY-417 baseline, update [`e2e-performance-baseline.md`](../learnings/e2e-performance-baseline.md) when you land non-trivial suite changes.

## Cross-references

- Decision tree "which layer should this test live in?" → [`testing-layers.md`](testing-layers.md)
- Layer-5 audit (moving existing E2Es down the pyramid) → [PSY-434](https://linear.app/psychic-homily/issue/PSY-434)
- CI workflow config → `.github/workflows/ci.yml`
- Global setup / teardown → `frontend/e2e/global-setup.ts`
- Fixtures (auth, error detection, cleanup) → `frontend/e2e/fixtures/`
- Backend test-only endpoint → `backend/internal/api/handlers/test_fixtures.go`
