# Contribution-surface audit — detail pages

> Audit note for PSY-489. Verifies the Phase-1 coverage table for three
> engagement-surfacing components across the 6 entity detail pages, maps the
> target state, and enumerates proposed follow-up tickets. No code changes.

## Scope

The three components under audit:

- **`AttributionLine`** (`frontend/features/contributions/components/AttributionLine.tsx`)
  "Last edited by {user} · {relative time}". Reads `/revisions/{entity_type}/{entity_id}?limit=1`. Renders null if no revisions exist.
- **`ContributionPrompt`** (`frontend/features/contributions/components/ContributionPrompt.tsx`)
  Data-gap nudge with dismiss. Reads `/entities/{entity_type}/{slug}/data-gaps`. Props are typed to `EditableEntityType = 'artist' | 'venue' | 'festival'` — releases/labels/shows cannot even compile-check.
- **`RevisionHistory`** (`frontend/components/shared/RevisionHistory.tsx`)
  Collapsible list of past revisions with field-level diffs and admin rollback. Reads `/revisions/{entity_type}/{entity_id}`.

The 6 detail-page files read for this audit:

- `frontend/features/shows/components/ShowDetail.tsx`
- `frontend/features/artists/components/ArtistDetail.tsx`
- `frontend/features/venues/components/VenueDetail.tsx`
- `frontend/features/releases/components/ReleaseDetail.tsx`
- `frontend/features/labels/components/LabelDetail.tsx`
- `frontend/features/festivals/components/FestivalDetail.tsx`

## Backend plumbing (authoritative)

Before interpreting coverage, pin down what each endpoint actually supports.

| Endpoint | Entity types actually supported |
|----------|---------------------------------|
| `GET /revisions/{entity_type}/{entity_id}` (reads) | Any `entity_type` accepted at the route level. But revisions are only **written** by handler paths that call `RecordRevision`. |
| `RecordRevision` callers (writes) | `artist` (`handlers/artist.go:865`), `venue` (`handlers/venue.go:531`), `festival` (`handlers/festival.go:358`), and indirectly through `pending_edit.go` approval (artist/venue/festival only). |
| `GET /entities/{entity_type}/{id_or_slug}/data-gaps` | Hardcoded switch: `artist`, `venue`, `festival` — any other type returns `400 Bad Request` (`handlers/data_gaps.go:67-75`). |
| `PUT /{entity}/suggest-edit` + `PendingEditEntityTypes` allow-list | `artist`, `venue`, `festival` only. `IsValidPendingEditEntityType("release") == false`, same for `show`, `label` (see `models/pending_entity_edit.go:48-60` and `services/admin/pending_edit_test.go:22-28`). |
| `PUT /labels/{label_id}` / `PUT /releases/{release_id}` | Exists, but **admin-only** (`requireAdmin`) and does **not** write to revisions. Labels/releases currently have no community-edit path and no revision trail. |

Implications:

- `AttributionLine` can technically mount on any entity. But on `release`/`label`/`show` it **always renders null** today — their update paths don't write revisions. It's dead wiring there, not a useful surface.
- `RevisionHistory` has the same behavior — will always be empty on `release`/`label`/`show` until those entities get a revision-writing edit path.
- `ContributionPrompt` is gated by TypeScript (`EditableEntityType`) AND the backend 400s it on non-editable types. Adding it to `release`/`label`/`show` requires backend changes first.

## Verified coverage table

| Component | Shows | Artists | Venues | Releases | Labels | Festivals |
|-----------|-------|---------|--------|----------|--------|-----------|
| `AttributionLine` | Absent — intentional | Present | Present | **Present** (but dead wiring — no revisions written) | Absent | Present |
| `ContributionPrompt` | Absent — intentional | Present | Present | Absent (backend + type block) | Absent (backend + type block) | Present |
| `RevisionHistory` | Absent — intentional | Present | Present | **Present** (but dead wiring — no revisions written) | Absent | Absent (drift — backend writes revisions for festivals) |

Re-verification against the Phase-1 table in PSY-489:

- Phase 1 marked Releases `ContributionPrompt` as `~`. **Actual: absent.** Cannot be added without backend support (type + endpoint both block it).
- Phase 1 marked Labels `RevisionHistory` as `~`. **Actual: absent.** Backend would return empty; also no edit surface.
- Phase 1 marked Festivals `RevisionHistory` as `~`. **Actual: absent.** Backend writes revisions for festivals (`handlers/festival.go:358`), so this is pure frontend drift — a real fill opportunity.
- Phase 1 marked Labels `AttributionLine` as "—" (absent). **Actual: absent.** Also consistent with backend (no label revisions).
- All other cells match.

## Per-component target state + reasoning

### `AttributionLine`

**Target:** render only on entities whose edit path actually writes revisions. Today that's artist, venue, festival.

| Entity | Recommendation | Reasoning |
|--------|---------------|-----------|
| Show | Absent (stay) | **Intentional** (PSY-461). Show edits flow through admin/owner-only inline form and status toggles, not the community revision trail. Documented as load-bearing. |
| Artist | Present (keep) | Backend records revisions for every `suggest-edit` approval and direct artist edit. |
| Venue | Present (keep) | Same as artist. |
| Release | **Present today but dead — remove or fix backend first.** | Component renders but returns null on every release page because releases never record revisions. Either wire the backend (release edit path writes revisions) or remove the visual-only noop from `ReleaseDetail.tsx`. |
| Label | Absent (stay) | Consistent with backend — labels have no revision trail. |
| Festival | Present (keep) | Backend records revisions for festivals. |

### `ContributionPrompt`

**Target:** only on entities with a suggest-edit path + data-gaps computation. Today that's artist, venue, festival.

| Entity | Recommendation | Reasoning |
|--------|---------------|-----------|
| Show | Absent (stay) | **Intentional** (PSY-461). Shows don't use community suggest-edit; show engagement is covered by `AttendanceButton`, `SaveButton`, `AddToCollectionButton`, `FieldNotesSection`. Adding a data-gap prompt would duplicate/conflict with show-specific engagement. |
| Artist | Present (keep) | Fully wired. |
| Venue | Present (keep) | Fully wired. |
| Release | Absent (stay, unless backend expands) | Cannot mount without backend data-gaps + pending-edit support for releases. **Ambiguous — flag for user**: is the plan to extend community editing to releases? If yes, file a backend ticket. If no, document the opt-out. |
| Label | Absent (stay, unless backend expands) | Same as release. |
| Festival | Present (keep) | Fully wired. |

### `RevisionHistory`

**Target:** render on entities with a write-path that records revisions. Today that's artist, venue, festival.

| Entity | Recommendation | Reasoning |
|--------|---------------|-----------|
| Show | Absent (stay) | **Intentional** (PSY-461). Shows have no revision trail. |
| Artist | Present (keep) | Fully wired. |
| Venue | Present (keep) | Fully wired. |
| Release | **Present today but dead — remove or fix backend first.** | Same as `AttributionLine`: renders a "History" collapsible that always shows "No edit history". |
| Label | Absent (stay) | Labels have no revision trail. |
| Festival | **Absent — drift. Should add.** | Festivals record revisions on edit (`handlers/festival.go:358`) and ship `AttributionLine` + `ContributionPrompt` + `EntityEditDrawer`. The absence of `RevisionHistory` looks like a miss when the other two components are present. Trivial wiring. |

## Delta list

### Recommended fills

1. **Festival: add `RevisionHistory`.** All backend plumbing exists; the component is already imported in peers (artist/venue). Mechanical drop-in below `EntityDetailLayout`, matching `ArtistDetail.tsx:1047-1053`.

### Recommended cleanups (remove dead wiring)

2. **Release: remove `AttributionLine` + `RevisionHistory`** (both render nothing today; remove the imports and JSX blocks so we don't ship a "History" collapsible that claims "No edit history" on every release). Alternative: leave in place if we expect to extend revision-writing to releases in the near term — flag for user decision.

### Documented opt-outs (in-code comment on the detail page)

3. **Show**: add a short comment above the render block noting that `AttributionLine` / `ContributionPrompt` / `RevisionHistory` are intentionally absent because show edits flow through admin/owner-only status toggles, not community suggest-edit. Cross-link to `docs/learnings/entity-detail-layout-migration.md` and this doc. Prevents future "why is this page different?" audit churn.
4. **Label**: add a short comment noting that labels have no community-edit surface today (admin-only update), so attribution/prompt/history don't apply. Cross-link.

### Ambiguous — flag for user decision

5. **Releases — should we extend community editing to releases?** If yes, file a backend ticket (pending-edit + revision-write for releases) + frontend follow-up to wire `ContributionPrompt` and rescue the existing (currently-dead) `AttributionLine` / `RevisionHistory`. If no, the recommended cleanup (item 2 above) applies.
6. **Labels — same question as releases.** Today labels have no community-edit surface at all.

These are not speculative design calls the agent should make. The product decision is whether the knowledge graph includes release/label edits on the community side or stays admin-only there. Treat as an open question until the user confirms.

## Proposed follow-up tickets (titles only — do not file until user greenlights)

- **`PSY-XXX: add RevisionHistory to FestivalDetail`** — mechanical fill; all plumbing exists. Smallest viable follow-up.
- **`PSY-XXX: document intentional contribution-surface opt-outs on ShowDetail and LabelDetail`** — in-code comments + docstring linking this audit, so future agents don't re-flag. Optional if we'd rather leave the audit note as the single source of truth.
- **`PSY-XXX: remove dead AttributionLine and RevisionHistory from ReleaseDetail`** — if we decide releases are admin-only going forward. Otherwise folded into the product ticket below.
- **`PSY-XXX: product decision — extend community editing to releases/labels?`** — user-facing product question. If the answer is yes, this becomes the umbrella ticket for backend pending-edit + data-gaps expansion + frontend wiring.

## Risks

- **Item 2 (remove dead wiring on releases)** is reversible but visible. If we ship it and then decide to add community editing to releases later, we just re-import the components. Low risk.
- **Item 1 (add RevisionHistory to festivals)** is pure addition. Low risk.
- None of the recommendations touch ShowDetail — PSY-461's intentional absence is preserved.

## Pointers for the follow-up PRs

- Reference fill pattern: `frontend/features/artists/components/ArtistDetail.tsx:1047-1053` (sibling below layout, `mt-0` wrapper, pass `isAdmin`).
- Reference prompt pattern: `frontend/features/festivals/components/FestivalDetail.tsx:333-342` (inside header slot after `EntityTagList`).
- Reference opt-out comment: `frontend/features/shows/components/ShowDetail.tsx:15` could gain a docstring block above the imports pointing to this audit.
