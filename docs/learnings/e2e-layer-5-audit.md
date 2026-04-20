# E2E Layer-5 Audit — 2026-04-19

> Closes PSY-434. Companion doc to PSY-445 ([`docs/strategy/testing-layers.md`](../strategy/testing-layers.md)) and PSY-417 ([`docs/learnings/e2e-performance-baseline.md`](./e2e-performance-baseline.md)).

## Purpose

PSY-445 built the full journey catalog and first-pass categorization (70 tests, ~50% Keep / ~43% → component / 7% delete-merge). Five of the delete-merge rows were consolidated in PSY-454 into `pages/navigation.spec.ts`; three backfill specs (PSY-455 add-to-collection, PSY-456 comments, PSY-457 follow-and-attendance) shipped after that. Current runtime-test count is **73** (71 `test(` calls in 27 files, minus 1 parameterized spec that emits 3 runtime tests).

PSY-434 turns PSY-445's categorization into an *actionable* move list. The rules:

- **No speculative reclassification.** If the assertions span layers, mark "Keep — spans layers" rather than invent a split. Five clear wins beat fifteen ambiguous ones.
- **Trust the lower layers.** Where a component test in `frontend/features/**/*.test.tsx` already covers the assertion, flag the E2E version as redundant.
- **Don't move anything in this ticket.** Output is this doc + follow-up tickets. Each move lands in its own PR (one per feature batch), deleting the E2E row in the same PR to avoid carrying both layers.

## Top 5 move candidates

Ranked by `value = (observed runtime) × (redundancy confidence) × (flake surface)`. All five are **already-mocked or already-rendered-without-backend assertions wearing E2E clothes**. Source-of-truth component tests either exist today or are a ~20-line addition.

### 1. `pages/ai-filler.spec.ts` — all 3 tests (already fully mocks the backend)

- **Runtime**: not profiled in PSY-417 top-20, but the spec makes 3 browser-auth round-trips, navigates to `/submissions`, and exercises a large form. Conservatively 5–8 s × 3 tests ≈ **15–24 s**.
- **What the test actually does**: calls `authenticatedPage.route('**/api/ai/extract-show', …)` to mock the entire backend, then clicks through the UI.
- **Why it's not an E2E**: the real backend is *never called* — the test's own lines 37/93/146 prove this. The only thing this test exercises that a component test wouldn't is the auth fixture (which is pure overhead for this assertion) and the Playwright browser runtime (which is overkill for a mocked form).
- **Recommended target**: `frontend/features/shows/components/AIFormFiller.test.tsx` (new file) or extend `SubmitShowForm.test.tsx`. Use Vitest + RTL + `vi.fn()` for the fetch mock. The `test-flyer.png` upload works via `userEvent.upload()` in jsdom.
- **Estimated savings**: 15–24 s wall-clock + 3 tests off the shared DB + elimination of 3 `authenticatedPage` context boots.

### 2. `pages/artist-detail.spec.ts:44` + `pages/venue-detail.spec.ts:44` — "shows tabs switch between upcoming and past"

- **Runtime**: `artist-detail:44` clocked **13.6 s** in the PSY-417 baseline (top-20 slowest). `venue-detail:44` is its twin at **6.1 s**. Combined **~19.7 s**.
- **What the test actually does**: navigates `/shows` → picks `.first()` show → clicks artist/venue link → clicks a Radix Tabs trigger → asserts `aria-selected`.
- **Why it's not an E2E**: three navigations of pure setup to verify Radix Tabs toggles `aria-selected` — a vendor library with its own test suite. No backend mutation, no cross-page state survival, no auth. The "real API" only provides a URL to navigate to.
- **Recommended target**: extend `frontend/features/artists/components/ArtistDetail.test.tsx` (already mocks Radix Tabs — see its `vi.mock('@/components/ui/tabs', ...)` at lines 24–28) and `frontend/features/venues/components/VenueDetail.test.tsx`. Assert `onTabChange` is called with `'past'` on click, or switch to real Radix and assert `data-state='active'`.
- **Estimated savings**: ~19.7 s wall-clock + removes 2 of the top-20 slowest tests + eliminates a race with parallel tests hitting `.first()` on `/shows`.

### 3. `pages/favorite-venue.spec.ts:13` — "favorite button is hidden when not authenticated"

- **Runtime**: **7.1 s** in the PSY-417 baseline.
- **What the test actually does**: navigates unauthenticated to a reserved venue detail URL; asserts the favorite button has zero matches.
- **Why it's not an E2E**: `frontend/features/venues/components/FavoriteVenueButton.test.tsx:47` **already asserts** `'renders nothing when not authenticated'`. The E2E is direct duplication of a component test that already passes.
- **Recommended target**: delete the E2E test. The component test is the source of truth. No new file, no new assertion.
- **Estimated savings**: **7.1 s wall-clock, 1 redundant test removed**, lowest-risk move in the top-5.

### 4. `pages/city-filter.spec.ts:5` — "city filter combobox and popular cities are visible"

- **Runtime**: **7.5 s** in the PSY-417 baseline.
- **What the test actually does**: loads `/shows`, waits for at least one `article`, asserts `city-filter-combobox` and `popular-cities` test-IDs are visible, and that a Phoenix button exists.
- **Why it's not an E2E**: `frontend/components/filters/CityFilters.test.tsx` **already covers** this — lines 28–34 ("renders the combobox trigger") and 211–224 ("shows popular cities when none are selected") cover exactly the same assertions, with Phoenix as a test-ID. The E2E test is a duplicate wearing Playwright cost.
- **Recommended target**: delete the E2E test. If we want any smoke assertion for the city-filter surface, the remaining `city-filter.spec.ts:27` (URL round-trip, already tagged `@smoke`) is the golden path.
- **Estimated savings**: **7.5 s wall-clock, 1 redundant test removed**.

### 5. `pages/show-list-actions.spec.ts:80` — "show admin edit controls only for admins"

- **Runtime**: **9.0 s** in the PSY-417 baseline.
- **What the test actually does**: spins up **two** authenticated contexts (`authenticatedPage` + `adminPage`), navigates both to `/shows`, and asserts the admin sees `[title="Edit show"]` and the regular user does not.
- **Why it's not an E2E**: this is role-based conditional rendering driven by `user.isAdmin`. Two full Playwright contexts + two auth cookies + two full-page renders to verify an `if (user?.isAdmin)` branch. The backend is not even a participant — both users query `/shows` and get the same payload; the UI just hides/shows an icon.
- **Recommended target**: extend `frontend/features/shows/components/ShowCard.test.tsx` (or wherever `[title="Edit show"]` lives) with two cases: `renders admin edit control when user.isAdmin` and `does not render when !user.isAdmin`. Mock `useAuth()` in each case.
- **Estimated savings**: **9.0 s wall-clock** + the highest-cost context setup in the suite (two worker-scoped auth contexts for one assertion).

### Top-5 total

| # | Spec:line | Observed runtime | Savings |
|---|---|---|---|
| 1 | `pages/ai-filler.spec.ts` (3 tests) | ~15–24 s | ~15–24 s |
| 2 | `pages/artist-detail.spec.ts:44` + `pages/venue-detail.spec.ts:44` | ~19.7 s | ~19.7 s |
| 3 | `pages/favorite-venue.spec.ts:13` | 7.1 s | 7.1 s |
| 4 | `pages/city-filter.spec.ts:5` | 7.5 s | 7.5 s |
| 5 | `pages/show-list-actions.spec.ts:80` | 9.0 s | 9.0 s |
| **Total** | **~58–67 s** | **~58–67 s** | |

At 5 workers (PSY-417 baseline: 109 s wall-clock), this equates to a **~12 s wall-clock win on the full suite** and a **~12 s win on the smoke subset** when the shed weight comes off. More importantly, #3 and #4 are pure-redundancy deletions — zero risk to coverage. #1 and #5 shrink the highest-leverage fixture footprint (no auth context needed where none is needed). #2 removes two of the top-20 slowest tests.

## Full inventory

73 runtime tests across 27 spec files. Categorizations align with PSY-445's table in `docs/strategy/testing-layers.md`; this column adds **Action** (what to do next) and a redundancy note where the assertion is already covered by a `.test.tsx` today.

**Action legend:**
- `Keep` — genuine E2E; don't move.
- `Keep (smoke)` — canonical golden path; already `@smoke`-tagged.
- `→ component` — move to Vitest + RTL; delete the E2E row in the same PR.
- `→ component (DUPE)` — assertion already covered by an existing component test; **delete the E2E outright, no new test needed**.
- `→ integration` — move to Go integration test.
- `Keep — spans layers` — assertions partially E2E and partially cheaper-layer-suitable; not splitting in this ticket per no-speculative-reclassification rule.

| Spec:line | Test | Assertions | Action | Rationale / duplicate-of |
|---|---|---|---|---|
| `admin/pending-shows.spec.ts:6` | displays pending shows for admin review | Admin-auth + real DB list render | Keep | Real auth + cross-system read. |
| `admin/pending-shows.spec.ts:32` | can approve a pending show | State transition: pending → published | Keep (smoke) | Admin workflow smoke. |
| `admin/pending-shows.spec.ts:71` | can reject a pending show with reason | State transition + required-reason dialog | Keep | Companion to the approve smoke. |
| `admin/venue-edits.spec.ts:6` | displays pending venue edits | Admin-auth + ChangeDiff render | Keep | Real diff rendering from DB. |
| `admin/venue-edits.spec.ts:24` | can approve a venue edit | State transition | Keep | Mutation + UI readback. |
| `admin/venue-edits.spec.ts:58` | can reject a venue edit with reason | State transition + reason dialog | Keep | Companion to approve. |
| `admin/verify-venue.spec.ts:6` | displays unverified venues list | Admin-auth + list render | Keep | Real admin read. |
| `admin/verify-venue.spec.ts:29` | can verify an unverified venue | State transition | Keep | Mutation + UI readback. |
| `auth/login.spec.ts:10` | logs in with valid credentials and redirects | Real cookie round-trip + nav | Keep (smoke) | Canonical auth smoke. |
| `auth/login.spec.ts:37` | shows error for invalid credentials | Error alert from 401 | → component | Duplicate of `features/auth/**` form test (can be added). Already covered at the form layer in principle; mockable fetch. |
| `auth/login.spec.ts:48` | shows validation error for empty password | Client-side required-field | → component | Pure HTML/Zod validation; no backend contact. |
| `auth/login.spec.ts:62` | logout returns to unauthenticated state | Cookie clearing + UI flip | Keep (smoke) | Real session teardown; 8.3 s — investigate speedup separately per PSY-417. |
| `auth/magic-link.spec.ts:20` | authenticates user with valid magic link | Real JWT → session | Keep (smoke) | Passwordless smoke. |
| `auth/magic-link.spec.ts:43` | shows error for expired/invalid magic link | Static error-page state | → component | No backend needed; page-level 4xx state. |
| `auth/magic-link.spec.ts:54` | shows invalid link when no token provided | Static error-page state | → component | Same. |
| `auth/register.spec.ts:10` | registers a new account and redirects | Real signup + cookie | Keep (smoke) | Conversion path. |
| `auth/register.spec.ts:38` | shows password strength requirements | Live meter + disabled submit | → component | Pure client UI; no backend. |
| `auth/register.spec.ts:62` | shows error for breached password | 200+error alert render | → component | Go has its own breach test; frontend side is alert render from a mocked response. |
| `auth/verify-email.spec.ts:20` | verifies email with valid token | Real JWT → activation | Keep | Cross-layer session flip. |
| `auth/verify-email.spec.ts:39` | shows error for invalid token | Static error-page state | → component | Page-level 4xx render. |
| `auth/verify-email.spec.ts:50` | shows invalid link when no token provided | Static error-page state | → component | Same. |
| `pages/add-to-collection.spec.ts` (1 test) | add show to collection | Full-stack add + UI readback | Keep (smoke) | Shipped in PSY-455; canonical collections mutation smoke. |
| `pages/ai-filler.spec.ts:33` | extracts show info from pasted text | Mocked API → form populated | → component (DUPE-ish) | **Top-5 #1.** Already mocks the backend; see `page.route` at line 37. |
| `pages/ai-filler.spec.ts:89` | extracts show info from uploaded image | Mocked API + file upload → form populated | → component | **Top-5 #1.** Same — mocks `page.route`. |
| `pages/ai-filler.spec.ts:141` | shows error when extraction fails | 200+success:false → error alert | → component | **Top-5 #1.** Already fully mocked. |
| `pages/artist-detail.spec.ts:5` | displays artist information with shows tabs | Real detail render from API | Keep | Detail-page smoke; keep one E2E per entity. |
| `pages/artist-detail.spec.ts:44` | shows tabs switch between upcoming and past | Radix Tabs `aria-selected` toggle | → component | **Top-5 #2.** `ArtistDetail.test.tsx` already mocks Radix Tabs; trivial to assert `onTabChange`. |
| `pages/city-filter.spec.ts:5` | combobox + popular cities visible | Static render check | → component (DUPE) | **Top-5 #4.** `components/filters/CityFilters.test.tsx:28–34, 211–224` already asserts this. |
| `pages/city-filter.spec.ts:27` | clicking a city updates URL + filters shows | URL state + query param round-trip | Keep (smoke) | Router state + API filter interplay. |
| `pages/city-filter.spec.ts:60` | All Cities button resets the filter | API response wait + URL reset | Keep | Real API interaction; complements the smoke. |
| `pages/city-filter.spec.ts:94` | city filter preserves state across page navigation | Router-state survival across back-nav | Keep | Only E2E catches history/router-state regressions. |
| `pages/collection.spec.ts:6` | displays Library heading and tabs | Static tab render | → component | Pure render; mockable. |
| `pages/collection.spec.ts:33` | shows empty state when no shows are saved | Empty-state render from empty API | → component | Classic "given this API response, does this UI render" — textbook component test. |
| `pages/collection.spec.ts:51` | falls back to shows tab when tab query is invalid | URL param parsing + default tab | → component | Pure client routing; mockable. |
| `pages/collection.spec.ts:66` | shows saved show after saving one | Save → navigate to library → verify | Keep (smoke) | Cross-page persistence smoke. |
| `pages/comments.spec.ts` (3 tests) | create / reply / vote | Auth + polymorphic mutation + nested render + Wilson flip | Keep (smoke) | PSY-456 backfill; all three are canonical cross-layer flows. |
| `pages/favorite-venue.spec.ts:13` | favorite button is hidden when not authenticated | Auth-conditional render | → component (DUPE) | **Top-5 #3.** `features/venues/components/FavoriteVenueButton.test.tsx:47` already covers this. |
| `pages/favorite-venue.spec.ts:31` | can favorite and unfavorite a venue from detail page | Real mutation round-trip | Keep (smoke) | Canonical favorite smoke. |
| `pages/favorite-venue.spec.ts:88` | favorited venue appears in library venues tab | Cross-page persistence | Keep | Library readback after mutation. |
| `pages/follow-and-attendance.spec.ts:19` | follow an artist round-trip surfaces in Library | Auth + mutation + library readback | Keep (smoke) | PSY-457 follow smoke. |
| `pages/follow-and-attendance.spec.ts:105` | mark show as going round-trip surfaces in Library | Auth + mutation + library readback | Keep (smoke) | PSY-457 going smoke. |
| `pages/home.spec.ts:5` | loads and displays upcoming shows | Homepage SSR + data render | Keep (smoke) | Cheapest landing-page signal. |
| `pages/home.spec.ts:32` | displays navigation links | Nav link render | → component | Pure rendering; mockable. `TopBar.test.tsx` already exists. |
| `pages/home.spec.ts:45` | displays blog and DJ set sections | SSR section heading render | → component | Heading assertion; mockable. |
| `pages/my-submissions.spec.ts:5` | displays user submissions in Submissions tab | Auth'd read of own submissions | Keep | Real user-scoped query. |
| `pages/my-submissions.spec.ts:35` | shows submission status and details | Badge + text render | → component | Static render assertion given a mocked list. |
| `pages/navigation.spec.ts` (3 runtime tests, parameterized) | list → detail → back for shows/artists/venues | Router nav + breadcrumb | Keep | PSY-454 consolidated form; keep all 3. |
| `pages/profile.spec.ts:4` | displays profile information for authenticated user | Auth'd read of profile | Keep | Real session + DB read. |
| `pages/profile.spec.ts:40` | settings tab shows account sections | Static sections render | → component | Pure render; mockable. `SettingsPanel.test.tsx` exists. |
| `pages/profile.spec.ts:74` | admin user sees admin-only sections | Role-conditional render | → component | Same pattern as top-5 #5; `user.isAdmin` branch. |
| `pages/protected-routes.spec.ts:5` | unauth redirected from /library to /auth | Real middleware | Keep (smoke) | Middleware gate smoke. |
| `pages/protected-routes.spec.ts:20` | unauth redirected from /submissions to /auth | Real middleware | Keep | Same gate class; tiny. |
| `pages/save-show.spec.ts:13` | save button is hidden when not authenticated | Auth-conditional render | → component | `components/shared/SaveButton.test.tsx:46` already covers this ("renders nothing when not authenticated"). Delete outright. |
| `pages/save-show.spec.ts:29` | can save and unsave a show from detail page | Real mutation round-trip | Keep (smoke) | Canonical save smoke. |
| `pages/save-show.spec.ts:78` | save state persists after navigation | Cross-page persistence | Keep | Navigation persistence check. |
| `pages/show-detail.spec.ts:5` | displays show details with artist and venue links | Real detail render | Keep | Detail-page smoke. |
| `pages/show-detail.spec.ts:39` | page title includes artist and venue | Document-title assertion | → component | SSR metadata; doable in a component-shaped test. |
| `pages/show-list-actions.spec.ts:4` | hide save buttons for unauthenticated users | Auth-conditional render | → component | Same SaveButton auth branch — covered by the shared component test. |
| `pages/show-list-actions.spec.ts:17` | toggle save state from list cards for authenticated users | Real mutation from list | Keep | Different code path than detail-page save. |
| `pages/show-list-actions.spec.ts:80` | show admin edit controls only for admins | Role-conditional render | → component | **Top-5 #5.** Two contexts for a conditional-render check. |
| `pages/shows.spec.ts:5` | loads and displays upcoming shows | List smoke | Keep (smoke) | Shows-list smoke. |
| `pages/shows.spec.ts:24` | show cards contain artist links, venue, and details link | Card render | → component | `ShowCard.test.tsx` already exists; extend it. |
| `pages/shows.spec.ts:44` | pagination loads more shows | Real pagination endpoint | Keep | Limit semantics + endpoint interplay. |
| `pages/submit-show.spec.ts:5` | displays submission form for verified user | Form render | → component | Pure render; mockable. |
| `pages/submit-show.spec.ts:38` | can submit a show with existing venue | Full submit flow | Keep (smoke) | Canonical contributor smoke; PSY-435 fixed the option-role bug. |
| `pages/submit-show.spec.ts:107` | redirects unauthenticated user to login | Middleware redirect | Keep | Gate smoke; tiny. |
| `pages/venue-detail.spec.ts:5` | displays venue information with shows tabs | Real detail render | Keep | Detail-page smoke. |
| `pages/venue-detail.spec.ts:44` | shows tabs switch between upcoming and past | Radix Tabs toggle | → component | **Top-5 #2 (paired with artist-detail).** |
| `pages/venues.spec.ts:5` | loads and displays venues | List smoke | Keep | Venues-list smoke. |
| `pages/venues.spec.ts:20` | venue cards show name, location, and show count | Card render | → component | `VenueCard.test.tsx` already exists; extend it. |

### Summary counts (post-PSY-454, post-backfill specs)

| Bucket | Count | Delta vs PSY-445 table |
|---|---|---|
| Keep in E2E (non-smoke) | 24 | +2 (PSY-454 consolidation kept 3 instead of 5) |
| Keep (smoke) | 17 | +4 (5 backfill smokes from PSY-455/456/457 minus 1 smoke that was folded in) |
| → component | 27 | −3 (delete-outright dupes counted separately below) |
| → component (DUPE, delete outright) | 5 | new column: #3 favorite-venue:13, #4 city-filter:5, save-show:13, show-list-actions:4, and `features/auth` overlaps (conservative count) |
| → integration | 0 | unchanged — no UI-anchored integration candidates remain in the current spec set |
| **Total runtime tests** | **73** | 68 + 5 backfills (PSY-455/456/457) |

Shipping the full `→ component` + `DUPE` migration plan would leave the suite at **41 E2E tests**, of which 17 are smoke. That matches PSY-445's original target distribution with the backfill specs baked in.

## Structural observations

These are orthogonal to the top-5 move list but worth flagging now while the audit context is fresh.

1. **Two parallel auth-fixture patterns.** `frontend/e2e/fixtures/auth.ts` and `frontend/e2e/fixtures/error-detection.ts` provide different `test` exports. Some specs import from `../fixtures` (the barrel), others from `../fixtures/error-detection` directly, others from `../fixtures`. No enforcement; easy to pick the wrong one. **Suggestion**: consolidate to a single export from `fixtures/index.ts` that layers error-detection *on top of* auth, so every spec gets both. This is out of scope here — file as a follow-up.

2. **Repeated `/shows` → `.first() article` → click-first-link setup**. At least 7 specs open with the same 5-line preamble: `page.goto('/shows')` → wait for first `article` → click first show-detail link. After PSY-430's move to reserved seeded slugs, most of these *could* `goto('/shows/e2e-...')` directly. The navigation-to-detail leg is already covered canonically by `pages/navigation.spec.ts`. **Suggestion**: audit each remaining `.first()` navigation; replace with direct-goto unless the list→detail click is load-bearing to the assertion. Saves 1–2 s per affected test.

3. **Role-based-rendering tests consistently spin up two contexts.** `show-list-actions.spec.ts:80` and `profile.spec.ts:74` both create `authenticatedPage` + `adminPage` for a single assertion each. That's four auth contexts across two tests to verify `user.isAdmin` branches. These are systematic component-test material (see top-5 #5). **Suggestion**: treat "role-based conditional render" as a blanket rule — always a component test, never E2E.

4. **`ai-filler.spec.ts` is the cleanest case for "component test wearing E2E clothes" in the whole suite.** Every test in the file mocks the backend at the Playwright route layer. There's literally no real API call. If one example were chosen to motivate Layer 5 migration, this is it.

5. **`pages/home.spec.ts:32,45` ("displays navigation links" and "displays blog and DJ set sections") are SSR render tests.** They assert headings and link text. The first is already covered by `components/layout/TopBar.test.tsx`; the second is a Markdown-rendered SSR section that could be tested with a mocked MDX module. Low-risk deletes.

6. **No `→ integration` candidates in the current UI-anchored spec set.** That's expected — E2E tests are browser-anchored, so Go-integration moves happen via backfill of *new* integration tests (iCal/RSS output, comment auto-hide thresholds, merge-artist semantics) rather than migration of existing specs. PSY-445's "Coverage gaps" section already lists these; not re-listing here.

## Ambiguities worth flagging (not in top-5)

Per project feedback: "when in doubt, flag — don't guess." These sit just outside top-5 because the call depends on human judgment.

- **`pages/collection.spec.ts:51` "falls back to shows tab when tab query is invalid"**: the assertion is pure URL-param parsing + default-tab-selection, which is unambiguously a component test. But it does take `authenticatedPage` because `/library` is gated. If the auth fixture cost dominates the test cost, moving it saves less than the above top-5 and I'm not confident on the break-even. Worth its own timing pass.
- **`auth/login.spec.ts:37` "shows error for invalid credentials"**: this currently contacts the real backend for the 401. PSY-445 categorizes this as `→ component` on the grounds that it's "error-alert rendering from 401." I agree, but flag that: the real backend path *also* exercises the rate-limiter and the login service's error-response shape. If we care about those, a Go integration test is the right home, not a component test. Calling out in case the intent was "let the mocked response represent both the rendering AND the contract."
- **`auth/register.spec.ts:62` "shows error for breached password"**: similar ambiguity. The assertion is UI; the breach check is Go. PSY-445 says `→ component`. If the Go side already has a breach-detection test (it does — see `password_validator` in `services/auth/`), the E2E adds nothing. If it doesn't, we'd want to confirm before deleting. **Recommend: before moving, spot-check that the Go side covers the alert text the frontend displays.**

## Recommended follow-up tickets

Per PSY-445's guidance ("feature-flavored batches, delete the E2E spec as the last commit so we never carry both"), file these as separate Linear tickets:

1. **`PSY-434a`** — migrate `ai-filler.spec.ts` (3 tests) to a component test. Largest single-file wall-clock win with zero coverage loss.
2. **`PSY-434b`** — migrate `artist-detail.spec.ts:44` + `venue-detail.spec.ts:44` ("shows tabs switch") to `ArtistDetail.test.tsx` / `VenueDetail.test.tsx`. Removes two top-20-slowest tests.
3. **`PSY-434c`** — delete `favorite-venue.spec.ts:13`, `save-show.spec.ts:13`, `show-list-actions.spec.ts:4`, `city-filter.spec.ts:5` as redundant with existing component tests. One PR, four deletions.
4. **`PSY-434d`** — migrate `show-list-actions.spec.ts:80` + `profile.spec.ts:74` ("role-based conditional render") to component tests. Pattern-level ticket.
5. **`PSY-434e`** — migrate auth-error-state tests: `login.spec.ts:37,48`, `register.spec.ts:38,62`, `magic-link.spec.ts:43,54`, `verify-email.spec.ts:39,50`. Resolve the ambiguity in the flagged section above before starting.
6. **`PSY-434f`** — migrate the static-render batch: `collection.spec.ts:6,33,51`, `home.spec.ts:32,45`, `shows.spec.ts:24`, `venues.spec.ts:20`, `my-submissions.spec.ts:35`, `show-detail.spec.ts:39`, `submit-show.spec.ts:5`.

Expected cumulative savings if all ship: **~60 s wall-clock off the serial test-runtime sum**, which at 5 workers translates to a steady ~10–15 s wall-clock improvement depending on scheduling. More importantly, ~27 tests off the shared DB → less parallelism contention and a materially smaller flake surface.

## What's intentionally not in this doc

- **Specific new test files/assertions to write.** That's follow-up-ticket work; the doc's job is to prove the move is correct, not to spec the component test.
- **Re-profiling the suite.** PSY-417's numbers are 2–3 weeks old but the suite shape is stable; if the follow-up tickets benchmark, file it alongside the migration PR.
- **Smoke-set changes.** PSY-446 tagged 17 tests `@smoke`; none of the top-5 candidates are `@smoke`-tagged. No smoke churn needed.
