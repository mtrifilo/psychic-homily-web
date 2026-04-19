# E2E Performance Baseline (PSY-417)

Captured **2026-04-19** on `main` at commit `a44f3e7` (post-PSY-433, pre-PSY-430 merge).

Goal: establish a baseline so PSY-418 (sharding) and later tickets can target the biggest levers with confidence. No code changes proposed here — this document only enumerates where the time goes and what's worth fixing.

## Headline numbers

| Metric                     | Value            |
| -------------------------- | ---------------- |
| **Total wall clock**       | **109 s**        |
| Global setup               | 14 s (~13%)      |
| Test execution + teardown  | 95 s (~87%)      |
| Tests run                  | 70               |
| Workers                    | 5                |
| Cumulative test-duration   | 372 s            |
| Effective parallelism      | ~3.9x            |
| Median per-test duration   | 4.3 s            |
| p75 / p95 per-test         | 6.1 s / 13.6 s   |

**Pass/fail breakdown on this run**

| Status    | Count | Cumulative ms |
| --------- | ----- | ------------- |
| passed    | 65    | 306,714       |
| failed    | 3     | 35,458        |
| timedOut  | 1     | 30,062        |
| skipped   | 1     | 0             |

The 4 non-passing tests are **all pre-existing known flakes** already covered by other tickets (see [Flake inventory](#flake-inventory)). They account for ~18% of total test-time.

## Phase breakdown

| Phase                                                 | Duration | Notes                                                                                       |
| ----------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------- |
| Docker network + Postgres container up                | ~6 s     | `docker compose up` + healthcheck wait.                                                     |
| `migrate` service run + wait                          | ~2 s     | Runs all migrations against the fresh DB.                                                   |
| `setup-db.sh` (seed)                                  | <1 s     | Pure SQL inserts.                                                                           |
| Go backend startup + `/health` wait                   | ~3 s     | Backend boot + container init; PSY-433 already stripped background-service startup noise.   |
| Frontend dev server reuse + healthcheck               | <1 s     | `reuseExistingServer: true`; no cold start on local runs with the dev server already up.    |
| Auth state capture for test users                     | ~1 s     | Sequential login for regular + admin users.                                                 |
| **Setup total**                                       | **14 s** | Derived from `stats.startTime` → first test `startTime`.                                    |
| Test execution (5 parallel workers)                   | ~93 s    | Wall clock from "Running 70 tests" to teardown start.                                       |
| Teardown (docker down + kill backend)                 | ~2 s     | Handled by `global-teardown.ts`.                                                            |

## Top-20 slowest individual tests

| ms      | file:line                                      | status    | title                                                       |
| ------- | ---------------------------------------------- | --------- | ----------------------------------------------------------- |
| 30,062  | pages/submit-show.spec.ts:34                   | timedOut  | can submit a show with existing venue **(flake, PSY-437)**  |
| 18,520  | pages/collection.spec.ts:66                    | failed    | shows saved show after saving one **(flake, PSY-430)**      |
| 13,771  | pages/artist-detail.spec.ts:5                  | passed    | displays artist information with shows tabs                 |
| 13,620  | pages/artist-detail.spec.ts:80                 | passed    | shows tabs switch between upcoming and past                 |
| 12,536  | pages/artist-detail.spec.ts:44                 | passed    | back to artists link navigates to artists list              |
| 11,781  | pages/save-show.spec.ts:32                     | failed    | can save and unsave a show from detail page **(flake, PSY-430)** |
| 11,057  | pages/city-filter.spec.ts:27                   | passed    | clicking a city in combobox updates URL and filters shows   |
|  9,366  | pages/city-filter.spec.ts:94                   | passed    | city filter preserves state across page navigation          |
|  9,358  | pages/show-list-actions.spec.ts:17             | passed    | toggle save state from list cards for authenticated users   |
|  8,980  | pages/show-list-actions.spec.ts:74             | passed    | show admin edit controls only for admins                    |
|  8,883  | pages/show-detail.spec.ts:62                   | passed    | back to shows link navigates to shows list                  |
|  8,251  | auth/login.spec.ts:62                          | passed    | logout returns to unauthenticated state                     |
|  7,471  | pages/city-filter.spec.ts:5                    | passed    | city filter combobox and popular cities are visible         |
|  7,100  | pages/favorite-venue.spec.ts:7                 | passed    | favorite button is hidden when not authenticated            |
|  6,763  | pages/venue-detail.spec.ts:44                  | passed    | back to venues link navigates to venues list                |
|  6,490  | pages/venue-detail.spec.ts:5                   | passed    | displays venue information with shows tabs                  |
|  6,347  | auth/magic-link.spec.ts:20                     | passed    | authenticates user with valid magic link                    |
|  6,077  | pages/venue-detail.spec.ts:76                  | passed    | shows tabs switch between upcoming and past                 |
|  5,772  | pages/city-filter.spec.ts:60                   | passed    | All Cities button resets the filter                         |
|  5,578  | pages/collection.spec.ts:51                    | passed    | falls back to shows tab when tab query is invalid           |

## Top-10 slowest test **files** (cumulative time)

| Total ms | Tests | File                                   |
| -------- | ----- | -------------------------------------- |
| 39,927   | 3     | pages/artist-detail.spec.ts            |
| 34,896   | 3     | pages/submit-show.spec.ts              |
| 33,666   | 4     | pages/city-filter.spec.ts              |
| 33,281   | 4     | pages/collection.spec.ts               |
| 21,811   | 3     | pages/show-list-actions.spec.ts        |
| 19,330   | 3     | pages/venue-detail.spec.ts             |
| 18,398   | 3     | pages/show-detail.spec.ts              |
| 17,656   | 3     | pages/favorite-venue.spec.ts           |
| 16,313   | 3     | pages/save-show.spec.ts                |
| 16,022   | 4     | auth/login.spec.ts                     |

## Flake inventory

All four non-passing tests on this run have tickets and are in-flight or merged:

| Test                                    | Ticket status                                |
| --------------------------------------- | -------------------------------------------- |
| submit-show.spec.ts:34 (Valley Bar)     | **PSY-437** — open (investigation)           |
| collection.spec.ts:66                   | **PSY-430** — PR open (fixed)                |
| save-show.spec.ts:32                    | **PSY-430** — PR open (fixed)                |
| favorite-venue.spec.ts:96               | **PSY-430** — PR open (fixed)                |

PSY-430 alone should flip ~35 s of test-time from red to green (and probably faster, since the fixed versions skip list-nav and go straight to a reserved row). PSY-437 saves another 30 s by replacing a timeout with a real pass.

## Speedup hypotheses (enumerate — do NOT implement here)

Grouped by expected impact. Numbers are rough estimates from the data above.

### 1. Merge PSY-430 + resolve PSY-437 (~35–60 s recovered)

- PSY-430's reserved-row approach makes the 4 flaky tests both fast **and** deterministic. Direct-URL navigation skips `/shows` list paint + article-card enumeration, which is ~2–3 s on its own.
- PSY-437 is the Valley Bar timeout; even if the fix preserves the test shape, moving from `timedOut (30,062 ms)` to `passed (~3 s)` is a pure win.
- **Expected impact:** ~10–15 s shaved off total wall clock once both land.

### 2. Higher worker count + sharding (PSY-418)

- Cumulative test-time is 372 s; wall clock for the test phase is 93 s. Effective parallelism is ~3.9x on 5 workers.
- CI uses 3 workers (per `playwright.config.ts`). Local uses 5. The drop to 3 on CI likely makes the test phase noticeably longer in CI — worth re-running the profile in CI to confirm.
- Sharding across 2 CI jobs with 3 workers each (or a single job bumped to 5–6 workers) is the most direct lever once the Layer 1–4 work is in.
- **Expected impact:** halving the test-phase wall clock is plausible given the `372 s / 93 s` ratio — with enough workers, ~45–50 s test phase is reachable.

### 3. Tighten slow "navigate-through-the-UI-to-test-something" patterns

Several files spend most of their time re-traversing `/shows` → `article.first()` → detail → drill deeper. The tests don't need the nav path; they're testing the destination page.

- `artist-detail.spec.ts` — 3 tests, 40 s total, all navigate `/shows → show detail → artist link`. Direct `page.goto('/artists/{slug}')` for the 2 tests that aren't specifically about the nav path would cut this roughly in half.
- `venue-detail.spec.ts` — same pattern, 19 s / 3 tests.
- `show-detail.spec.ts` — same, 18 s / 3 tests.
- **Expected impact:** ~20–30 s of cumulative test-time reclaimed, ~5–8 s off wall clock depending on worker packing.

### 4. Reuse a persistent Postgres container

- Setup is 14 s; the Docker Postgres boot + migrate step is most of it (~8 s).
- A long-lived test DB container plus a `TRUNCATE` pass between runs (or a `pg_dump` / `pg_restore` baseline) would skip the boot entirely.
- **Expected impact:** shaves ~8–10 s off setup. Makes iterative local E2E runs cheaper too.

### 5. Collapse `auth/login.spec.ts` duration

- 4 tests, 16 s cumulative. The `logout` test alone is 8.25 s.
- The `authenticatedPage` / `adminPage` fixtures already cache auth state; these tests exercise the raw login/logout path and shouldn't be a bottleneck, but `logout returns to unauthenticated state` (8.25 s) is worth a quick look — may have an over-budget wait hiding a 1-s real interaction.
- **Expected impact:** minor (~3–5 s), but easy if the fix is a locator change.

### 6. Reduce list-visit cold-paint cost

- `/shows` list paint + article render is hit by ~20 tests as a nav starting point. Even 500 ms of cost per visit adds up.
- If the dev server isn't warmed for `/shows` route compilation before tests start, the first worker's first `/shows` visit pays a Next.js route-compile tax.
- Worth measuring: hit `/shows` once in `global-setup.ts` after frontend healthcheck, before auth capture, to prime the Next dev compilation cache.
- **Expected impact:** unknown; needs measurement. Could be 2–5 s off first-test latency.

## Non-impact observations (call-outs, not levers)

- **152 occurrences of `timeout: 10_000`** across the test files. This sounds like over-budget waiting but Playwright only blocks up to the timeout on *failure* — on success, the wait returns as soon as the condition is met. So the 10 s isn't the actual wait time. No action needed.
- **Test duration distribution**: 2 tests < 1 s, 20 tests 1–3 s, 21 tests 3–5 s, 20 tests 5–10 s, 7 tests > 10 s. Most of the suite is in the "healthy" 1–5 s range. The long tail of 7 tests > 10 s is where attention pays off.
- **Teardown is already tight** (~2 s). Not worth optimizing.

## Notes on this capture

- Captured via `PLAYWRIGHT_JSON_OUTPUT_NAME=/tmp/psy417-e2e.json bun run test:e2e -- --reporter=json` from `frontend/`.
- Ran with the default local config (5 workers, `fullyParallel: true`).
- Dev server (`bun run dev`, port 3000) was already running; `playwright.config.ts` sets `reuseExistingServer: true` so frontend cold start is excluded from this baseline. A CI run would add ~10–20 s for `bun run dev` startup.
- Post-PSY-433: all seven `DISABLE_*` env flags active, so the backend log is clean of radio-fetch / auto-promotion chatter. This did not measurably change wall-clock on this run (setup was already tight) but it did remove log spam that would have blown up artifact sizes on CI retries.

## What this unblocks

- **PSY-418** — sharding, with real numbers to inform shard count.
- **PSY-436** — E2E scaling strategy decision record. Update that doc after Layers 1–4 land so it reflects real state rather than assumed state.
- **PSY-411** — enable E2E on PRs. Blocked on PSY-418 landing a CI wall-clock inside budget (~10 min total per hand-off).
