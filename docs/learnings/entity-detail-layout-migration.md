# EntityDetailLayout migration — ShowDetail (and VenueDetail)

> Research note for PSY-461. Produced before any code changes. Documents every
> current divergence between the 4 layout-based detail pages and the 2
> hand-rolled ones (ShowDetail, VenueDetail), classifies each divergence as
> load-bearing or drift, and recommends a migration path + scope.

## Background

We have 6 entity detail pages. Four use the shared `EntityDetailLayout`
template, two are hand-rolled:

| Page            | Layout base                          | Notes |
|-----------------|--------------------------------------|-------|
| `ArtistDetail`  | `EntityDetailLayout`                 | Tabs: Overview / Discography / Labels. Sidebar. |
| `ReleaseDetail` | `EntityDetailLayout`                 | Tabs: Overview / Listen-Buy. Cover-art sidebar. |
| `LabelDetail`   | `EntityDetailLayout`                 | Tabs: Overview / Roster / Catalog. Details sidebar. |
| `FestivalDetail`| `EntityDetailLayout`                 | Tabs: Lineup / Insights / Series / Info. Flyer sidebar. |
| `ShowDetail`    | Hand-rolled `<div>` + `<header>`     | No tabs; flat vertical sections. |
| `VenueDetail`   | Hand-rolled 2-col grid (map on right)| No tabs; shows list in main column. |

`EntityDetailLayout` contract (`frontend/components/shared/EntityDetailLayout.tsx`):

- Container: `container max-w-6xl mx-auto px-4 py-6`
- Breadcrumb: fed by `{ fallback, entityName }`
- Header slot: arbitrary `ReactNode` rendered inside a `<header className="mb-6">`
- Tabs: mandatory — `tabs[]`, `activeTab`, `onTabChange`, children are
  `TabsContent` panels
- Sidebar: optional `ReactNode` rendered in an `<aside>` beside the main
  column on `lg:`

Everything outside the layout (revision history, comments, edit drawer,
report dialog) is rendered as siblings below `EntityDetailLayout` on the 4
layout-based pages. Those pages fragment-wrap `<>` the layout with trailing
siblings.

Headers on the 4 layout-based pages share a convention:

```tsx
<EntityHeader title=… subtitle=… actions=… />
<AttributionLine entityType=… entityId=… />
<EntityTagList entityType=… entityId=… isAuthenticated=… />
<ContributionPrompt ... />   // artist, festival, venue only
```

`EntityHeader` itself is simple: h1 + optional subtitle row + optional
actions on the right, with `sm:flex-row sm:items-start sm:justify-between`.

## ShowDetail divergences

Enumerated against a line-by-line read of
`frontend/features/shows/components/ShowDetail.tsx` (current tip of `main`).

| # | Divergence | Load-bearing or drift | Rationale |
|---|------------|-----------------------|-----------|
| 1 | Flat sections, no tabs | **Load-bearing** | Shows are transient single-event pages; there are only ~3 post-header sections (music embeds, field notes, comments). Forcing tabs adds a click to reach primary content. Other entity pages have multi-surface content that benefits from tabs (discography, roster, catalog). A single-tab layout is silly. |
| 2 | Cancelled `<Alert>` banner above header | **Load-bearing** | The cancellation state is safety-critical on a show page. It stays above-the-fold even when the header gets tall. Not drift. |
| 3 | Custom header block with bill-position artist rendering (h1 for headliners, "w/ …" for support, `(special guest)` inline) | **Load-bearing** | Uses show-specific `show_artists.set_type` semantics. Can't be crammed into `EntityHeader`'s single-string `title`. Needs its own sub-component. |
| 4 | Large venue link + `MapPin` + "See more shows at {venue} →" under the h1 | **Load-bearing** | The venue is the co-primary entity on a show page (Shows → Venue is a top navigation path). Degrading it to a subtitle badge (as `EntityHeader.subtitle` implies) hides it. |
| 5 | Show meta row (time / price / age) as `gap-x-4 gap-y-1 text-sm` | **Load-bearing** | Three discrete, conditional facts. Fits naturally under the header in the current shape; forcing it into `EntityHeader.subtitle` would mix it with h1-adjacent badges. |
| 6 | Inline "Buy Tickets" external CTA | **Load-bearing** | Primary conversion surface on upcoming shows. Surfacing it prominently is a product decision; can be kept in the show-specific header sub-component. |
| 7 | Description paragraph right below metadata | **Drift** | Other pages use `EntityDescription` inside a tab (artist/venue) or inline in the overview tab (release/label). But since ShowDetail intentionally has no tabs, inline is fine. Not meaningful drift — flagging for completeness. |
| 8 | `EntityTagList` inside header (PSY-439) | **Already aligned** | Added during PSY-439. Same placement as the 4 layout-based pages. |
| 9 | Right-column action buttons: `AttendanceButton`, `SaveButton`, `AddToCollectionButton`, `ReportShowButton`, admin `Edit`/`Delete`, status toggles (`Mark Sold Out`, `Mark Cancelled`) | **Load-bearing shape** | `EntityHeader.actions` is a single ReactNode — it can host this cluster. The `AttendanceButton` is on its own row above the flex-wrap button row; the status toggles are a second sub-row. Not a show-stopper, just shape work. |
| 10 | Inline edit form (not `EntityEditDrawer`) | **Drift (deferred)** | ShowDetail uses the older `<ShowForm mode="edit">` component inline; artist/venue/festival use `EntityEditDrawer` from `features/contributions`. Migrating ShowDetail to the drawer is a separate concern (new form + different edit-suggestion pathway) and out of scope for a layout refactor. Call it out, leave it alone. |
| 11 | `Artist Music Section` — "Listen to the Artists" heading + per-artist `MusicEmbed` | **Load-bearing** | Unique to shows; no other entity page aggregates per-artist embeds inline. Keep as a show-specific section sibling of the layout. |
| 12 | `EntityCollections` inline (not in sidebar) | **Drift** | The other 4 pages put `EntityCollections` in the sidebar. Because ShowDetail will not have a sidebar (no tabs, no sidebar needed — see recommendation below), inline placement is fine. Reclassify as load-bearing-given-no-sidebar. |
| 13 | `FieldNotesSection` | **Load-bearing** | Show-specific. Stays. |
| 14 | `CommentThread` as a normal section (no `mt-0 px-4 md:px-0` wrapper) | **Drift** | Other pages use the `mt-0 px-4 md:px-0` wrapper; ShowDetail does not. Trivial fix. |
| 15 | No `RevisionHistory` | **Intentional** | Shows don't use `RevisionHistory` today. Not drift — show edits go through a different pathway (admin-only status toggles + edit-form). Leave alone. |
| 16 | No `AttributionLine` | **Intentional** | Shows don't surface "last edited by" because edits are admin-driven. Leave alone. |
| 17 | No `ContributionPrompt` | **Intentional** | Shows don't use the community edit prompt (edit pathway is owner/admin, not community). Leave alone. |

## VenueDetail divergences

From `frontend/features/venues/components/VenueDetail.tsx`.

| # | Divergence | Load-bearing or drift | Rationale |
|---|------------|-----------------------|-----------|
| 1 | `max-w-5xl` instead of `max-w-6xl` | **Drift** | No documented reason; 5xl is narrower than the template. Either adopt 6xl or document why venues are narrower. Flagging for decision. |
| 2 | 2-column `grid-cols-[1fr_400px]` with map sidebar `order-1 lg:order-2` (map above header on mobile) | **Load-bearing shape** | The venue map sidebar on desktop is parallel to `EntityDetailLayout`'s sidebar. But the mobile ordering ("map first") is deliberate for wayfinding. Migrating preserves desktop shape for free; mobile ordering is a prop knob `EntityDetailLayout` doesn't have today. See below. |
| 3 | `BadgeCheck` inline with h1 for `verified` venues, `FavoriteVenueButton` / `FollowButton` / `AddToCollectionButton` / `NotifyMeButton` adjacent to h1 | **Drift** | `EntityHeader.actions` is the right home for the button cluster. Inline verified badge matches `EntityHeader.subtitle`. This is just re-shaped work. |
| 4 | No tabs, shows list in main column | **Load-bearing** | Same argument as ShowDetail (5) — venues have one primary content surface (upcoming shows). `EntityDetailLayout` requires tabs today. Adding a `tabs=[]` escape hatch OR accepting a single-tab layout is a layout-level question. See Migration Path B below. |
| 5 | Website link rendered as `domain.tld ↗` under h1 | **Load-bearing** | Natural EntityHeader.subtitle content. |
| 6 | `ContributionPrompt` below header (in main column, not in header slot) | **Drift** | Artist/festival put `ContributionPrompt` inside the header slot. Venues should too. |
| 7 | `EntityDescription` inline in the main column | **Drift** | Artist uses `EntityDescription` inside the overview tab. If venues get tabs, move into an overview tab. If not, leave inline. |
| 8 | `VenueLocationCard` + `VenueGenreProfile` + `EntityCollections` in right column | **Load-bearing** | Matches the `EntityDetailLayout` sidebar slot exactly. Easy wiring. |
| 9 | `RevisionHistory` + `CommentThread` as siblings below the grid | **Already aligned** | Same pattern as the 4 layout-based pages; zero change. |

## The "no tabs" blocker

Both ShowDetail and VenueDetail have flat, single-content-surface main
columns. `EntityDetailLayout` today **requires** `tabs` + `activeTab` +
`onTabChange`, and wraps children in `<Tabs>` with a `<TabsList>`. Rendering
a single-tab shell with one trigger labeled "Overview" is ugly UX — visual
noise for zero navigation value. It's also how release pages look when
there's nothing on "Listen/Buy" — one lonely tab labeled "Overview", which
is arguably drift that just hasn't been flagged.

Migration options:

**A. Extract a tabless variant of `EntityDetailLayout`** — a sibling
component, or a new prop like `tabs?: EntityDetailTab[]` where omitting
tabs skips the `<Tabs>` wrapper entirely. Small change to the layout; both
ShowDetail and VenueDetail migrate cleanly; the 4 existing pages are
unaffected.

**B. Add an auto-hide-tabs behavior** — if `tabs.length <= 1`, skip the
`TabsList` render but keep the `<Tabs>` wrapper (so `<TabsContent>` in
children still works). Lower diff but slightly magical.

**C. Don't migrate** — accept two different page shapes.

Recommend **A**, but it's a one-line layout change (`{tabs.length > 0 &&
<TabsList>...</TabsList>}` + gate the `<Tabs>` wrapper) plus updating the
prop type to make `tabs` optional. Very cheap.

## Proposed migration shape

### ShowDetail

Decompose into:

1. **`ShowHeader`** (new sub-component, `features/shows/components/ShowHeader.tsx`):
   - Owns date/status badges row
   - Owns bill-position artist rendering (h1 headliners + "w/ …" support)
   - Owns venue prominence block (name link + MapPin row + "see more shows" link)
   - Owns show meta row (time / price / age)
   - Owns ticket URL CTA
   - Owns description paragraph
   - Takes `show` as its only prop; pure presentation.

2. **`ShowActions`** (new sub-component):
   - Top row: `AttendanceButton`
   - Second row: `SaveButton` + `AddToCollectionButton` + `ReportShowButton` + admin Edit + owner/admin Delete
   - Third row (admin/owner only): status toggles (`Mark Sold Out`, `Mark Cancelled`)
   - Takes show + auth context + the three handlers (`onEdit`, `onDelete`, the
     two toggle mutations).

3. **`ShowDetail`** (migrated):
   - Renders cancelled alert banner (above `EntityDetailLayout`)
   - Renders `EntityDetailLayout` with:
     - `header={<><ShowHeader show={…} actions={<ShowActions ... />} /><EntityTagList .../></>}`
     - `tabs={[]}` (relies on the Migration Path A change above)
     - `sidebar` omitted
   - Keeps show-specific sections as siblings below the layout: edit form
     (when open), `Artist Music Section`, `EntityCollections`, `FieldNotesSection`,
     `CommentThread`.
   - Delete dialog stays as a portal sibling.

Net effect: the hand-rolled 250-line header block becomes `<ShowHeader>` +
`<ShowActions>`, the page plugs into the shared breadcrumb / container /
tabs-area contract, and future shared-layout changes (header spacing, tag
wrapper, etc.) propagate automatically.

### VenueDetail

Similar decomposition:

1. **`VenueHeader`**: h1 + verified badge + city/state line + website link.
2. **`VenueActions`**: the button cluster (`FavoriteVenueButton`,
   `FollowButton`, `AddToCollectionButton`, `NotifyMeButton`, admin
   Edit/Delete, `Report`).
3. **`VenueDetail`** migrated:
   - `header={<><VenueHeader /><AttributionLine .../><EntityTagList .../><ContributionPrompt .../></>}`
   - `tabs={[]}` (same single-surface case)
   - `sidebar={<><VenueLocationCard /><VenueGenreProfile /><EntityCollections /></>}`
   - `RevisionHistory` + `CommentThread` as siblings below.
   - Container width: pick `max-w-5xl` vs `max-w-6xl` — recommend `max-w-6xl`
     for parity unless venues have a specific reason for narrower. Flag in
     PR body either way so reviewer confirms.

Mobile ordering (map-above-header) is the only thing we'd lose. Two ways
to preserve:

- Leave it alone for now. The map is present either way; mobile readers
  will just see the header first. Acceptable.
- Add a `sidebarPosition?: 'leading' | 'trailing'` + `mobileFirst?: boolean`
  prop to the layout. Overkill for one venue page; defer until a second
  caller needs it.

## Scope recommendation

The ticket explicitly asks for this to be called out. Three options from
the ticket:

- **(a)** Scope to ShowDetail only; file a sibling ticket for VenueDetail.
- **(b)** Single research note covers both; separate PRs for each migration
  (this PR is ShowDetail, follow-up is VenueDetail).
- **(c)** Single PR covering both.

**Recommendation: (b).** Reasoning:

1. The research is genuinely shared — the `EntityDetailLayout` "no tabs"
   blocker is the same, the decomposition pattern (`EntityHeader` + a
   separate `EntityActions` cluster) is the same, and the classification
   of "load-bearing vs drift" uses the same yardstick. Writing it twice
   would duplicate this document.
2. But the code diffs are genuinely independent: ShowDetail has show-specific
   concerns (bill-position artists, field notes, attendance) that VenueDetail
   doesn't; VenueDetail has the 2-column grid + map sidebar + `max-w-5xl`
   decision that ShowDetail doesn't. A single PR would be >500 LOC of
   churn across four test suites and two route pages. Reviewer risk >
   benefit.
3. Two smaller PRs also let us catch visual regressions on one surface at a
   time.
4. (c) is the wrong bet even if the diffs look small — this is a
   heavily-visited component path, and bundling the risk doubles the
   regression surface without buying anything.

**Open scope decision for user:** confirm (b) and file a follow-up ticket
`PSY-XXX: migrate VenueDetail to EntityDetailLayout` after this PR merges.
This design note covers both; the follow-up ticket can link to this doc
for context.

## Migration path summary

1. **Layout change**: make `tabs` optional in `EntityDetailLayout` and
   skip the `<Tabs>` wrapper when `tabs.length === 0`. One-line gate +
   prop optionality. Add a vitest case for the no-tabs branch.
2. **ShowDetail** (this PR, PSY-461):
   - Extract `ShowHeader` + `ShowActions` sub-components.
   - Rewrite the main component to render on `EntityDetailLayout` with
     `tabs={[]}` and no sidebar.
   - Keep the cancelled alert banner above the layout.
   - Keep show-specific sections (music, collections, field notes, comments)
     as siblings below the layout.
   - Update `ShowDetail.test.tsx` with an `EntityDetailLayout` mock matching
     the pattern in `ArtistDetail.test.tsx`.
3. **VenueDetail** (follow-up ticket):
   - Extract `VenueHeader` + `VenueActions`.
   - Migrate to `EntityDetailLayout` with `tabs={[]}`, sidebar = current
     right column.
   - Resolve `max-w-5xl` vs `max-w-6xl` decision.
   - Mobile ordering: either accept loss or add a layout knob; recommend
     accept for now.

## Risks & mitigations

- **Visual regression on /shows/{slug}** — medium. Mitigation: decompose
  first without moving to `EntityDetailLayout` (just extract `ShowHeader`
  + `ShowActions` as pure presentational pieces and keep current
  container). Verify tests. Then flip the outer container to
  `EntityDetailLayout`. Two commits on the PR.
- **Tests that assert on DOM structure** — low. The existing ShowDetail
  tests are content-based (`screen.getByText(…)`, `screen.getByTestId(…)`),
  so layout swaps don't break them. The one structural assertion is the
  breadcrumb `navigation` role, which the new layout also renders.
- **Mobile map ordering for VenueDetail** — low. Deferred to follow-up.
- **PSY-439 regression**: `EntityTagList` is already in ShowDetail's header
  block; keeping it in the header slot of the migrated component preserves
  PSY-439.

## Acceptance criteria (for the ShowDetail PR)

Copied from the ticket + refined with this design:

- [ ] `EntityDetailLayout` accepts a no-tabs shape (one-line gate +
  prop-type change)
- [ ] `ShowDetail` renders on `EntityDetailLayout` with extracted
  `ShowHeader` + `ShowActions`
- [ ] Cancelled alert banner still rendered (above the layout)
- [ ] Artist Music Section, In Collections, Field Notes, Discussion all
  still render as siblings below the layout
- [ ] `EntityTagList` still in the header slot (no PSY-439 regression)
- [ ] `bunx vitest run features/shows` passes
- [ ] `bunx tsc --noEmit -p tsconfig.json` has no new errors on touched files
- [ ] PR body calls out the remaining decision: VenueDetail follow-up
  ticket to file
