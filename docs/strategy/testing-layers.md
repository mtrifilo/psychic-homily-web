# Testing Layers — Psychic Homily

> Closes PSY-445. Companion doc to PSY-417 ([`docs/learnings/e2e-performance-baseline.md`](../learnings/e2e-performance-baseline.md)) and feeds smoke-selection (PSY-446) and component-migration (PSY-434).

## Purpose

Three concrete questions this doc answers:

1. **What are the essential user journeys we care about?** — a persona-organized catalog, so "is X covered?" becomes a grep, not an argument.
2. **What's the right layer for each journey?** — E2E is precious (slow, serial DB state, expensive to debug); most UI behavior can live in component tests with the backend mocked; backend correctness lives in Go integration/unit tests.
3. **Where do the existing ~70 E2E tests fit?** — for each spec, does it stay in E2E, move down a layer, get deleted, or graduate to smoke?

Context: PSY-417 established that the full E2E suite runs in ~109 s locally with 5 workers. That's workable but fragile. Every E2E test we add taxes CI wall-clock and introduces new opportunities for environmental flake (Docker DNS, dev-server compile, parallel DB writes). We want E2E to be the thin top of a pyramid, not a catch-all.

## Layer definitions

### E2E (Playwright, real browser + real backend + real DB)

Use when the value of the test is the **integration between layers** — a real HTTP cookie, a real GORM write, the optimistic UI flipping before the backend responds, or a cross-page navigation that spans SSR + client fetch. E2E answers "does the full production-shaped stack behave correctly end-to-end?"

**Cost:** slow (seconds per test), stateful (shared DB, serial tests where writes conflict), hard to debug (Docker + network + parallel workers).

**Rule of thumb:** if mocking the backend with `page.route('**/api/...')` would make the test cheaper **without losing the thing you're verifying**, it's not an E2E test — it's a component test.

### Component test (Vitest + React Testing Library, real browser primitives, mocked backend)

Use for UI behavior: does the form validate, does the button transition between states, does the empty state render, does a loading skeleton appear, does an error alert show the right text? Use MSW or TanStack Query fixtures to mock network calls.

**Cost:** cheap (ms per test), isolated (no shared state), fast feedback loop.

**Rule of thumb:** if the test is "given this API response, does this UI render correctly?" — it's a component test.

### Integration test (Go `testcontainers` + real Postgres)

Use for backend business logic: does the service correctly compute Wilson scores, does the approval workflow transition a show to published, does a GORM query return the right joined rows? Uses `postgres:18` via testcontainers, full migrations run.

**Cost:** moderate (testcontainers boot is ~2 s, tests are fast after). Parallel-safe.

**Rule of thumb:** if the test is "given this DB state and this service call, does the right row change?" — it's an integration test.

### Unit test (Go or Vitest, no I/O)

Pure functions: formatters, slug generators, validators, reducers, URL builders, parsers. Nil-DB error paths.

**Cost:** effectively free.

**Rule of thumb:** if the test has no I/O, it's a unit test.

## How to pick a layer (quick decision tree)

```
My test needs a real backend AND a real browser
  → E2E (real auth cookie, cross-layer flake you want to catch)

My test needs a real browser but backend can be mocked
  → Component (Vitest + RTL + mock fetch)

My test is "given this DB state, does this service return X"
  → Go integration test (testcontainers)

My test is pure logic, no I/O
  → Unit test
```

If a candidate E2E test boils down to "assert UI renders correctly when the API returns this shape," the backend is a convenience, not a requirement — move it down.

## Golden user journeys catalog

Journeys are grouped by persona. Each row is a journey a user must be able to complete. Coverage reflects current state as of 2026-04-19. "Risk if untested" is honest: some journeys are very valuable; others are nice-to-have.

**Coverage values:** `E2E` (covered by one or more Playwright specs) · `component` (covered by a `.test.tsx` under `frontend/features/` or `frontend/app/`) · `unit` · `uncovered` (no automated test today).

**Recommended layer values:** `E2E (stays)` · `component` · `integration` (Go) · `mixed` (multiple layers).

### Unauthenticated visitor

| Journey | 1-line description | Current coverage | Recommended layer | Risk if untested |
|---|---|---|---|---|
| Landing page loads | Homepage renders upcoming shows, blog, DJ sets | E2E (home.spec.ts) | E2E (stays) | Homepage 500 or blank state silently breaks acquisition funnel. |
| Global nav visible | Top nav shows Shows/Venues/Blog/DJ Sets + Login | E2E (home.spec.ts) | component | Pure UI — mocking auth state in a component test would catch this just as well. |
| Browse shows list | `/shows` paginates, shows cards with links | E2E (shows.spec.ts) | E2E (stays) | List endpoint + SSR interplay; cheap, keep in E2E. |
| Pagination "Load More" | Click loads a second page of results | E2E (shows.spec.ts:44) | E2E (stays) | Real list-endpoint pagination semantics are worth covering. |
| City filter combobox | Pick a city → URL updates → results filter | E2E (city-filter.spec.ts) | E2E (stays) | Exercises real URL state + query param round-trip. |
| City filter persists across nav | Filter stays applied after visiting show detail + back | E2E (city-filter.spec.ts:94) | E2E (stays) | Router state survival — only E2E catches this. |
| Browse venues list | `/venues` renders cards with name/location/show count | E2E (venues.spec.ts) | E2E (stays) | Cheap; real API shape. |
| Browse artists list | `/artists` renders | uncovered | component | No list-specific E2E; list page behavior is shape assertion. |
| Browse releases list | `/releases` renders | uncovered | component | Same as artists. |
| Browse labels list | `/labels` renders | uncovered | component | Same. |
| Browse festivals list | `/festivals` renders | uncovered | component | Same. |
| Browse collections (public) | `/collections` browse with filters, search, tabs | uncovered | component | Browse UI is large and mockable; one smoke-path E2E would be enough. |
| Browse scenes | `/scenes` index + detail pages | uncovered | component | No automated coverage; detail page pulls 3 entity types. |
| Browse tags | `/tags` index + detail pages | uncovered | component | Tag enrichment landed in PSY-438 with zero E2E. |
| Browse charts | `/charts` trending/popular/hot pages | uncovered | component | Derived-data rendering; easy to mock. |
| Browse radio | `/radio` stations/shows/episodes/playlists | uncovered | component | Four nested index types; mock API responses. |
| Show detail page | Artist/venue links, breadcrumbs, page title | E2E (show-detail.spec.ts) | mixed | Keep one E2E smoke; nav-through-list tests can become component. |
| Artist detail page | Tabs, breadcrumb, upcoming/past shows | E2E (artist-detail.spec.ts) | mixed | Nav-through-list adds cost without signal; direct-goto variant could be component. |
| Venue detail page | Tabs, breadcrumb | E2E (venue-detail.spec.ts) | mixed | Same pattern as artist-detail. |
| Release detail page | Artist roles, external links | uncovered | component | No automated coverage of detail pages for 3 out of 6 core entities. |
| Label detail page | Roster, catalog | uncovered | component | Same. |
| Festival detail page | Lineup, overlap analysis | uncovered | component | Same. |
| Search via Cmd+K | Keyboard palette across 7 entity types | uncovered | mixed | Keyboard affordance + debounced multi-entity search; at least one E2E worth having. |
| iCal feed download | `/api/ical/...` returns valid VEVENTs | uncovered | integration | Format correctness is a Go integration test. |
| RSS feed | `/api/rss/...` returns valid XML | uncovered | integration | Same. |
| Collection browse (unauth) | Public collections visible anonymously | uncovered | component | Mockable. |
| Protected route redirect | `/library`, `/submissions` redirect unauth → `/auth` | E2E (protected-routes.spec.ts) | E2E (stays) | Must exercise real middleware; keep. |

### New account

| Journey | 1-line description | Current coverage | Recommended layer | Risk if untested |
|---|---|---|---|---|
| Email/password registration | Signup form → account created → logged in | E2E (register.spec.ts) | E2E (stays) | Critical conversion path; real auth cookie round-trip. |
| Password strength meter | Live unmet-requirements feedback + disabled submit | E2E (register.spec.ts:38) | component | Pure UI validation. |
| Breached password rejection | Server rejects known-breached password | E2E (register.spec.ts:62) | component | Assertion is "error alert shows" — mockable; Go side has its own test. |
| Email verification happy path | Click verified JWT link → success state | E2E (verify-email.spec.ts) | E2E (stays) | Real JWT + real session flip. |
| Email verification invalid | Garbage token → error page | E2E (verify-email.spec.ts:39) | component | No backend needed — page-level error state from a 4xx response. |
| Email verification missing token | No token → invalid-link state | E2E (verify-email.spec.ts:50) | component | Static page state. |
| Login happy path | Valid creds → authenticated state | E2E (login.spec.ts) | E2E (stays) | Canonical smoke. |
| Login invalid creds | Wrong password → error alert | E2E (login.spec.ts:37) | component | Error-alert rendering from 401. |
| Login validation empty password | Client-side required-field block | E2E (login.spec.ts:48) | component | Pure client validation. |
| Logout | Dropdown → sign-out → login link reappears | E2E (login.spec.ts:62) | E2E (stays) | Cookie-clearing round-trip; good smoke. |
| Magic link happy path | JWT token → authenticated + redirect | E2E (magic-link.spec.ts) | E2E (stays) | Exercises passwordless auth end-to-end. |
| Magic link expired/invalid | Bad token → error page | E2E (magic-link.spec.ts:43) | component | Static page state. |
| Magic link missing token | No token → invalid-link state | E2E (magic-link.spec.ts:54) | component | Static page state. |
| OAuth (Google) | Redirect → callback → authenticated | uncovered | uncovered | Hard to automate; rely on manual QA + monitoring; accept uncovered. |
| OAuth (GitHub) | Same | uncovered | uncovered | Same. |
| Passkey enrollment | WebAuthn create | partial component | component | `passkey-signup.test.tsx` exists; WebAuthn mock boundary. |
| Passkey login | WebAuthn get | partial component | component | `passkey-login.test.tsx` exists; same boundary. |

### Authenticated user

| Journey | 1-line description | Current coverage | Recommended layer | Risk if untested |
|---|---|---|---|---|
| Save a show (detail page) | Click "Add to My List" → button flips, persists | E2E (save-show.spec.ts) | E2E (stays) | Optimistic UI + real write; canonical smoke. |
| Save state persists across nav | Leave + return, state preserved | E2E (save-show.spec.ts:78) | E2E (stays) | Real persistence check. |
| Save from list card | Toggle save from `/shows` list | E2E (show-list-actions.spec.ts:17) | E2E (stays) | Different code path than detail. |
| Favorite a venue | Click Add to Favorites → persists | E2E (favorite-venue.spec.ts) | E2E (stays) | Canonical smoke. |
| Favorited venue appears in library | Library venues tab shows favorited entry | E2E (favorite-venue.spec.ts:88) | E2E (stays) | Cross-page persistence check. |
| Favorite button hidden (unauth) | Unauth users don't see the button | E2E (favorite-venue.spec.ts:7) | component | Conditional rendering based on auth state — mockable. |
| Save button hidden (unauth) | Same on shows | E2E (save-show.spec.ts:13, show-list-actions.spec.ts:4) | component | Same. |
| Follow an artist | Click Follow → button flips | E2E (follow-and-attendance.spec.ts) | E2E (stays) | PSY-56; PSY-457 backfilled follow-artist smoke. |
| Follow a venue | Same for venues | uncovered | component | Venue follow code path is identical to artist; cover once in E2E (artist) + component for venue-specific UI. |
| Going/Interested on show | PSY-55 attendance toggle | E2E (follow-and-attendance.spec.ts:106) | E2E (stays) | PSY-457 backfilled going smoke. |
| Comment on entity | Create a comment on show/artist/venue/etc | uncovered | mixed | Comments has 5 component tests for form/thread rendering; no E2E for the full create→view→moderation loop. |
| Reply to a comment | Nested reply (depth ≤ 3) | uncovered | component | Thread rendering is component-testable; one E2E smoke adequate. |
| Vote on a comment | Upvote/downvote, Wilson score update | uncovered | integration | Score math is a Go test; button-flip is component. |
| Field note on past show | Create field note with ratings/spoiler/verified | uncovered | mixed | Component tests exist for form/card rendering; end-to-end create→display loop uncovered. |
| Add to collection | Add a show/artist/etc to a collection | E2E (add-to-collection.spec.ts) | E2E (stays) | Full-stack flow shipped in PSY-314; smoke backfilled in PSY-455. |
| Remove from collection | Remove via collection detail page | uncovered | component | UI assertion after mocked DELETE. |
| Reorder collection items | Up/down buttons reorder items | uncovered | component | Pure UI; mock backend. |
| Per-item notes | Add/edit a note on a collection item | uncovered | component | Inline edit; mockable. |
| Create a new collection | From "Add to Collection" button | uncovered | E2E (stays) | Ownership + creator linking; worth a smoke. |
| Submit a show (existing venue) | Fill form, pick existing venue, submit | E2E (submit-show.spec.ts:34, currently flaking per PSY-437) | E2E (stays) | Core contributor flow. |
| Submit a show (new venue) | Same but with new-venue path | uncovered | E2E (stays) | Branch not covered; moderate risk. |
| Submit show unauth redirect | `/submissions` redirects unauth → `/auth` | E2E (submit-show.spec.ts:107) | E2E (stays) | Already tiny; keep. |
| Submit form visible | Form renders for verified user | E2E (submit-show.spec.ts:5) | component | Pure render check; mockable. |
| AI form filler (text) | Paste text → extract → form populated | E2E (ai-filler.spec.ts) | component | Test already mocks the API — no real backend dependency. |
| AI form filler (image) | Upload image → extract → form populated | E2E (ai-filler.spec.ts:89) | component | Same — already fully mocked. |
| AI form filler error | Server returns failure → error alert | E2E (ai-filler.spec.ts:141) | component | Already mocks to 200+success:false; component-grade test wearing E2E clothes. |
| My library (Shows tab) | Default tab shows saved shows | E2E (collection.spec.ts) | E2E (stays) | Core retention surface. |
| My library (empty state) | Empty library renders CTA | E2E (collection.spec.ts:33) | component | Pure empty-state rendering. |
| My library (invalid tab param) | Bad `?tab=` falls back to Shows | E2E (collection.spec.ts:51) | component | URL parsing + tab state; mockable. |
| My submissions tab | Shows user's submitted shows | E2E (my-submissions.spec.ts) | E2E (stays) | Real query over auth'd user's content. |
| Submission status rendering | Published/Pending badges | E2E (my-submissions.spec.ts:35) | component | Pure render assertion. |
| View own profile | `/profile` renders user info | E2E (profile.spec.ts) | E2E (stays) | Auth'd read of sensitive data. |
| Edit own profile | PATCH username/name/bio (PSY-261) | uncovered | component | Form submission with mocked API. |
| Profile settings tab | Email verification, change password, export, danger zone | E2E (profile.spec.ts:40) | component | Static sections; mockable. |
| Admin profile sections | API tokens + CLI auth visible | E2E (profile.spec.ts:74) | component | Role-based conditional rendering. |

### Contributor

| Journey | 1-line description | Current coverage | Recommended layer | Risk if untested |
|---|---|---|---|---|
| Suggest entity edit (drawer) | Open drawer on artist/venue/festival, submit edit | uncovered | E2E (stays) | PSY-127; shipped; zero automated coverage. |
| Report an entity | Flag an artist/venue/festival/comment | uncovered | component | Form submission; component-testable. |
| Report a comment | Flag a comment (auto-hide on 3+) | uncovered | integration | Auto-hide threshold is Go logic. |
| Create a collection | From collections browse or entity page | uncovered | E2E (stays) | Creator linkage + listing; smoke-worthy. |
| Edit own collection | Rename, change description, featured-toggle | uncovered | component | Form-only. |
| Edit own comment | Edit an existing comment | uncovered | component | Form-only. |
| View contribution history | `/contribute` or profile tab with own stats | uncovered | component | Derived data display. |
| View leaderboard | `/contribute/leaderboard` renders rankings | uncovered | component | Pure rendering; mock API. |
| Contributor profile page | Public `/users/{username}` with collections/stats | uncovered | component | Public-profile rendering; mockable. |
| Activity heatmap | Contribution calendar visualization | uncovered | component | Pure rendering. |
| Contribution prompt | CTA banner on entity page | partial component | component | `ContributionPrompt.test.tsx` exists. |

### Admin

| Journey | 1-line description | Current coverage | Recommended layer | Risk if untested |
|---|---|---|---|---|
| View pending shows | `/admin/pending-shows` lists seeded pending shows | E2E (pending-shows.spec.ts) | E2E (stays) | Admin auth + real data; worth keeping. |
| Approve a pending show | Approve dialog + result | E2E (pending-shows.spec.ts:32) | E2E (stays) | State transition end-to-end. |
| Reject a pending show with reason | Reject dialog + required reason + result | E2E (pending-shows.spec.ts:71) | E2E (stays) | State transition end-to-end. |
| View pending venue edits | `/admin/venue-edits` lists edits | E2E (venue-edits.spec.ts) | E2E (stays) | Admin + diff rendering. |
| Approve a venue edit | Approve dialog + result | E2E (venue-edits.spec.ts:24) | E2E (stays) | State transition. |
| Reject a venue edit with reason | Reject dialog + result | E2E (venue-edits.spec.ts:58) | E2E (stays) | State transition. |
| View unverified venues | `/admin/unverified-venues` lists venues | E2E (verify-venue.spec.ts) | E2E (stays) | Admin auth. |
| Verify a venue | Verify dialog + result | E2E (verify-venue.spec.ts:29) | E2E (stays) | State transition. |
| Batch approve/reject shows (PSY-81) | Multi-select + bulk action | uncovered | E2E (stays) | Shipped feature; missing smoke. |
| Moderate comments (pending queue) | Approve/reject queued comments | uncovered | E2E (stays) | PSY-292/293; shipped; zero automated coverage. |
| Hide/restore a comment | Admin toggle on a visible comment | uncovered | integration | Trust-tier visibility logic belongs in Go tests. |
| Handle reports queue | Admin reviews flagged entities/comments | uncovered | E2E (stays) | Cross-admin workflow; smoke-worthy. |
| Discovery imports UI | Trigger an import run | uncovered | component | Mockable; extraction pipeline has its own tests. |
| Radio station/matching admin | View/manage stations, approve matches | uncovered | component | Mockable. |
| Tag admin (merge/rename) | Tag administration page | uncovered | component | CRUD with mocked API. |
| Data quality dashboard | `/admin/data-quality` renders | uncovered | component | Pure rendering. |
| Merge artists (PSY-47) | Select + merge, destructive | uncovered | integration | DB-level correctness; Go test for the merge semantics. |
| Split artists | Same, reverse | uncovered | integration | Same. |
| Admin stats / analytics page | Platform analytics view | uncovered | component | Display layer on cached data. |
| Audit log | `/admin/audit-log` renders recent events | uncovered | component | Display layer. |

## Coverage gaps

Flagged below with **[backfill]** for gaps worth filing follow-up tickets. Don't file them from this doc — let the human triage.

- Follow system (artist/venue): PSY-56 — artist covered by `follow-and-attendance.spec.ts` (PSY-457); venue-specific UI falls to a component test per PSY-434.
- Going/Interested on shows: PSY-55 — covered by `follow-and-attendance.spec.ts` (PSY-457).
- **[backfill]** Collections mutation flows: create-collection — add-to-collection now covered by `e2e/pages/add-to-collection.spec.ts` (PSY-455); the inline "create new collection from picker" path still has no automated coverage beyond unit/component-level hooks.
- **[backfill]** Comments: entire feature (create, reply, vote, edit, report) has zero E2E; component tests exist for rendering but not for the full loop.
- **[backfill]** Field notes: structured-data flow uncovered end-to-end.
- **[backfill]** Entity edit drawer (PSY-127): community edit suggestions uncovered.
- **[backfill]** Comment moderation queue (PSY-292/293): admin-side uncovered.
- **[backfill]** Tag detail pages (PSY-438 just shipped): no E2E.
- **[backfill]** Cmd+K command palette (PSY-257): no E2E.
- **[backfill]** iCal/RSS feeds: no integration tests for generated output.
- **[backfill]** Batch approve/reject shows (PSY-81): no E2E.
- **[backfill]** Detail pages for releases/labels/festivals: no detail-page E2E for half of the core entities.
- **[uncovered, accept]** OAuth (Google/GitHub): hard to automate through the external provider redirect; rely on monitoring + manual QA.

## Existing E2E test categorization

Sorted by file. Timings from PSY-417 (top-20 only); others marked "not profiled" — they're in the healthy 1–5 s band per the distribution in the baseline doc.

**Legend:**
- **Stays** — real browser + backend required; maps to a golden journey that only makes sense end-to-end.
- **→ component** — UI behavior, no real backend needed.
- **→ integration** — backend behavior with UI scaffolding.
- **Delete/merge** — redundant, obsolete, or covered elsewhere.
- **Smoke** — Stays + critical golden-path; candidate for PSY-446 smoke-on-PR selection.

| file:line | test title | ms | categorization | rationale |
|---|---|---|---|---|
| admin/pending-shows.spec.ts:6 | displays pending shows for admin review | not profiled | Stays | Admin-auth gated + server-seeded list; real DB read. |
| admin/pending-shows.spec.ts:32 | can approve a pending show | not profiled | Smoke | State transition across admin workflow — flagship smoke for admin path. |
| admin/pending-shows.spec.ts:71 | can reject a pending show with reason | not profiled | Stays | Same transition class; keep under full admin suite. |
| admin/venue-edits.spec.ts:6 | displays pending venue edits | not profiled | Stays | Admin-auth + ChangeDiff render from real data. |
| admin/venue-edits.spec.ts:24 | can approve a venue edit | not profiled | Stays | State transition end-to-end. |
| admin/venue-edits.spec.ts:58 | can reject a venue edit with reason | not profiled | Stays | State transition end-to-end. |
| admin/verify-venue.spec.ts:6 | displays unverified venues list | not profiled | Stays | Admin-auth list render. |
| admin/verify-venue.spec.ts:29 | can verify an unverified venue | not profiled | Stays | State transition. |
| auth/login.spec.ts:10 | logs in with valid credentials and redirects to home | not profiled | Smoke | Core auth smoke; real cookie round-trip. |
| auth/login.spec.ts:37 | shows error for invalid credentials | not profiled | → component | Error-alert rendering from 401; mockable. |
| auth/login.spec.ts:48 | shows validation error for empty password | not profiled | → component | Pure client validation. |
| auth/login.spec.ts:62 | logout returns to unauthenticated state | 8,251 | Smoke | Cookie-clearing round-trip worth keeping; investigate the 8 s runtime per PSY-417 speedup hypothesis 5. |
| auth/magic-link.spec.ts:20 | authenticates user with valid magic link | 6,347 | Smoke | Passwordless smoke; real JWT + session flip. |
| auth/magic-link.spec.ts:43 | shows error for expired/invalid magic link | not profiled | → component | Static error-page state. |
| auth/magic-link.spec.ts:54 | shows invalid link when no token provided | not profiled | → component | Static error-page state. |
| auth/register.spec.ts:10 | registers a new account and redirects to home | not profiled | Smoke | Core conversion path. |
| auth/register.spec.ts:38 | shows password strength requirements | not profiled | → component | Pure client UI behavior. |
| auth/register.spec.ts:62 | shows error for breached password | not profiled | → component | 200+error path mocked; Go has its own breach test. |
| auth/verify-email.spec.ts:20 | verifies email with valid token | not profiled | Stays | Real JWT → session activation. |
| auth/verify-email.spec.ts:39 | shows error for invalid token | not profiled | → component | Static error-page state. |
| auth/verify-email.spec.ts:50 | shows invalid link when no token provided | not profiled | → component | Static error-page state. |
| pages/ai-filler.spec.ts:33 | extracts show info from pasted text | not profiled | → component | Test already mocks `/api/ai/extract-show` — no real backend dependency. |
| pages/ai-filler.spec.ts:89 | extracts show info from uploaded image | not profiled | → component | Same — fully mocked; file upload works in jsdom/happy-dom. |
| pages/ai-filler.spec.ts:141 | shows error when extraction fails | not profiled | → component | Already mocks 200+success:false — classic component test. |
| pages/artist-detail.spec.ts:5 | displays artist information with shows tabs | 13,771 | Stays | Keep one smoke that proves the detail page renders from real API. |
| pages/artist-detail.spec.ts:44 | back to artists link navigates to artists list | 12,536 | Delete/merge | Pure nav assertion; redundant with show-detail nav test; could merge into one cross-entity nav test or drop. |
| pages/artist-detail.spec.ts:80 | shows tabs switch between upcoming and past | 13,620 | → component | Tabs widget behavior; mockable. |
| pages/city-filter.spec.ts:5 | city filter combobox and popular cities are visible | 7,471 | → component | Render assertion; mockable. |
| pages/city-filter.spec.ts:27 | clicking a city in combobox updates URL and filters shows | 11,057 | Smoke | URL state + query param round-trip; keep as smoke. |
| pages/city-filter.spec.ts:60 | All Cities button resets the filter | 5,772 | Stays | Real API interaction on filter clear. |
| pages/city-filter.spec.ts:94 | city filter preserves state across page navigation | 9,366 | Stays | Router-state survival is E2E-only. |
| pages/collection.spec.ts:6 | displays Library heading and tabs | not profiled | → component | Static tab rendering. |
| pages/collection.spec.ts:33 | shows empty state when no shows are saved | not profiled | → component | Empty-state render. |
| pages/collection.spec.ts:51 | falls back to shows tab when tab query is invalid | 5,578 | → component | URL parsing + tab state; mockable. |
| pages/collection.spec.ts:66 | shows saved show after saving one | 18,520 | Smoke | Canonical save→verify-in-library loop; currently flaking per PSY-430 — fix lands and it stays. |
| pages/favorite-venue.spec.ts:7 | favorite button is hidden when not authenticated | 7,100 | → component | Auth-conditional rendering. |
| pages/favorite-venue.spec.ts:31 | can favorite and unfavorite a venue from detail page | not profiled | Smoke | Canonical venue-favorite loop. |
| pages/favorite-venue.spec.ts:88 | favorited venue appears in library venues tab | not profiled | Stays | Cross-page persistence verification. |
| pages/home.spec.ts:5 | loads and displays upcoming shows | not profiled | Smoke | Landing-page smoke; cheapest possible signal. |
| pages/home.spec.ts:32 | displays navigation links | not profiled | → component | Pure nav rendering. |
| pages/home.spec.ts:45 | displays blog and DJ set sections | not profiled | → component | SSR section render from markdown; assert a heading — mockable. |
| pages/my-submissions.spec.ts:5 | displays user submissions in Submissions tab | not profiled | Stays | Real query over auth'd user's submissions. |
| pages/my-submissions.spec.ts:35 | shows submission status and details | not profiled | → component | Badge + text render — mockable. |
| pages/profile.spec.ts:4 | displays profile information for authenticated user | not profiled | Stays | Auth'd read — real session + DB. |
| pages/profile.spec.ts:40 | settings tab shows account sections | not profiled | → component | Static sections; mockable. |
| pages/profile.spec.ts:74 | admin user sees admin-only sections | not profiled | → component | Role-conditional rendering. |
| pages/protected-routes.spec.ts:5 | unauthenticated user is redirected from /library to /auth | not profiled | Smoke | Real middleware; tiny; canonical gate smoke. |
| pages/protected-routes.spec.ts:20 | unauthenticated user is redirected from /submissions to /auth | not profiled | Stays | Same gate class; keep. |
| pages/save-show.spec.ts:13 | save button is hidden when not authenticated | not profiled | → component | Auth-conditional rendering. |
| pages/save-show.spec.ts:29 | can save and unsave a show from detail page | 11,781 | Smoke | Canonical save/unsave loop; currently flaking per PSY-430 — stays once fixed. |
| pages/save-show.spec.ts:78 | save state persists after navigation | not profiled | Stays | Navigation persistence check. |
| pages/show-detail.spec.ts:5 | displays show details with artist and venue links | not profiled | Stays | Detail-page render from real API. |
| pages/show-detail.spec.ts:39 | page title includes artist and venue | not profiled | → component | Document-title assertion; SSR metadata doable in a component-like setup. |
| pages/show-detail.spec.ts:62 | back to shows link navigates to shows list | 8,883 | Delete/merge | Pure nav; redundant with artist/venue-detail nav tests; consolidate or drop. |
| pages/show-list-actions.spec.ts:4 | hide save buttons for unauthenticated users | not profiled | → component | Auth-conditional rendering. |
| pages/show-list-actions.spec.ts:17 | toggle save state from list cards for authenticated users | 9,358 | Stays | Real save-from-list path; distinct from detail-page save. |
| pages/show-list-actions.spec.ts:74 | show admin edit controls only for admins | 8,980 | → component | Role-based conditional rendering. |
| pages/shows.spec.ts:5 | loads and displays upcoming shows | not profiled | Smoke | Shows-list smoke. |
| pages/shows.spec.ts:24 | show cards contain artist links, venue, and details link | not profiled | → component | Card rendering. |
| pages/shows.spec.ts:44 | pagination loads more shows | not profiled | Stays | Real pagination endpoint + limit semantics. |
| pages/shows.spec.ts:71 | show detail link navigates correctly | not profiled | Delete/merge | Nav-only; overlaps show-detail.spec.ts coverage. |
| pages/submit-show.spec.ts:5 | displays submission form for verified user | not profiled | → component | Form render; mockable. |
| pages/submit-show.spec.ts:34 | can submit a show with existing venue | 30,062 (timedOut) | Smoke | Core contributor smoke; blocked by PSY-437 flake investigation. |
| pages/submit-show.spec.ts:107 | redirects unauthenticated user to login | not profiled | Stays | Gate smoke; tiny. |
| pages/venue-detail.spec.ts:5 | displays venue information with shows tabs | 6,490 | Stays | Detail render from real API. |
| pages/venue-detail.spec.ts:44 | back to venues link navigates to venues list | 6,763 | Delete/merge | Pure nav; redundant with other back-link tests. |
| pages/venue-detail.spec.ts:76 | shows tabs switch between upcoming and past | 6,077 | → component | Tabs widget behavior. |
| pages/venues.spec.ts:5 | loads and displays venues | not profiled | Stays | Venues-list smoke. |
| pages/venues.spec.ts:20 | venue cards show name, location, and show count | not profiled | → component | Card rendering. |
| pages/venues.spec.ts:40 | venue name links to detail page | not profiled | Delete/merge | Pure nav; consumed by `venue-detail.spec.ts:5`. |

### Summary counts

| Bucket | Count |
|---|---|
| Stays in E2E (non-smoke) | 22 |
| Smoke candidate (Stays + critical golden path) | 13 |
| → component | 30 |
| → integration | 0 |
| Delete/merge | 5 |
| **Total categorized** | **70** |

Total matches the PSY-417 baseline's 70-test count. Adding Stays + Smoke gives 35 tests kept in E2E (50%), 30 migrated to component (43%), 5 deleted/merged (7%). That's the lever PSY-434 is set up to pull.

Nothing was categorized as `→ integration` because the existing specs are all UI-anchored; the Go-integration opportunities live in the **Coverage gaps** section above (iCal/RSS, comment vote scoring, comment auto-hide, merge/split semantics).

## Recommended follow-ups

- **PSY-434** (component-migration): the `→ component` rows in the categorization table are the menu. Recommend migrating in **feature-flavored batches** (e.g., all auth→component in one PR, all ai-filler in one PR, all `.tabs switch` tests in one PR) — each batch should delete the E2E spec as its last commit so we never carry both.
- **PSY-446** (smoke-on-PR): the 13 **Smoke** rows are the starting selection. Budget target: <60 s wall-clock on PR CI. If that's tight, drop down to 6–8 by preferring one smoke per persona (landing, register, login, save-show, favorite-venue, approve-pending-show).
- **[backfill candidates]** The "Coverage gaps" list names ~13 shipped features with no E2E. The highest-value backfill candidates (real-user-impact × shipped-but-unverified):
  1. ~~**Collections add-to-collection flow** (PSY-314, shipped, no coverage — PMF-critical feature).~~ Addressed by PSY-455 (`e2e/pages/add-to-collection.spec.ts`, tagged `@smoke`).
  2. **Comments create + reply + vote** (Wave 1–5, shipped, no coverage — community moat).
  3. ~~**Follow / Going-Interested** (PSY-55, -56, shipped, no coverage — cheap smoke).~~ Addressed by PSY-457 (`follow-and-attendance.spec.ts`).

## How to add new tests (quick reference)

```
Does my test need a real backend?
  └─ YES → does it also need a real browser?
      └─ YES → E2E (frontend/e2e/...). Consider smoke if it's a golden journey.
      └─ NO  → Go integration test (backend/internal/services/..._integration_test.go).
  └─ NO  → does it need a real browser?
      └─ YES → component test (next to the component, *.test.tsx).
      └─ NO  → unit test (Go or Vitest).
```

When in doubt, **write the cheapest test that provides the signal you want**. A component test that flakes 0.1% and runs in 40 ms beats an E2E test that flakes 2% and runs in 8 s, even if the E2E test is "more realistic."

### Smell checks when you're about to write an E2E test

- Do I mock the API in this test? → component test.
- Do I only assert on rendered text / visible elements? → component test.
- Is the backend incidental (I'd mock it if I could, but this is where my harness lives)? → fix the harness, then component test.
- Is this exercising a cross-layer concern: real auth cookie, real DB state survival, SSR+client hydration interplay, middleware redirect? → E2E.
