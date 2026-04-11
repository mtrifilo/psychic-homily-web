# Collaborative Entity Descriptions — Design Doc

> **STATUS: SHIPPED (March 2026).** Entity descriptions added to artists and venues (PSY-211–215). Retained as reference.

## Problem

PH entity pages show structured data (name, location, tags, relationships) but no **narrative context**. Users visiting an artist page can see their upcoming shows and genre tags, but not *who this band is* or *why they matter*. Venues have addresses but no *vibe description*. This is a knowledge graph, not just a database — narrative is what makes it a source of truth.

What.cd's artist wikis were community-maintained narratives with full edit history. They were distinct from comments (discussion) and distinct from structured data (year, label, format). PH needs the same layer.

## Decisions

### 1. Treat descriptions as regular entity fields (not separate wiki tables)

**Decision:** Descriptions are a `TEXT` column on each entity table, edited through the same Phase 3 pending edit system as any other field.

**Why not separate wiki tables (Gazelle pattern)?**
- Gazelle needed separate tables because torrent metadata (format, bitrate) was fundamentally different from free-text descriptions
- PH's entities are simpler — a description is just another field like `name` or `city`
- Phase 3's `pending_entity_edits` with JSONB `field_changes` already handles arbitrary field edits
- RevisionService already tracks per-field changes with old/new values
- Separate tables would mean separate permissions, separate moderation queue, separate revision history — unnecessary complexity

**What we gain:** Descriptions automatically get pending edit queue, revision history, rollback, attribution, and trust-tiered permissions for free — no new infrastructure needed.

### 2. Content format: Markdown

**Decision:** Descriptions use Markdown (same as profile sections).

**Why:**
- Already used for profile sections (`UserProfileSection.Content`)
- Frontend already has Markdown rendering (used in profile, request descriptions, collection descriptions)
- Richer than plain text, simpler than rich text editor
- Familiar to the technical/music community PH targets
- No BBCode (Gazelle's choice was a product of its era)

**Constraints:**
- Max 5,000 characters (prevents abuse, encourages conciseness)
- No embedded images in Markdown (security) — use external image URLs only
- Sanitized on render (XSS prevention)

### 3. Entity scope and priority

| Entity | Has `description` column? | Priority | Rationale |
|--------|--------------------------|----------|-----------|
| **Artist** | No — needs migration | **P0** | "Who is this band?" is the #1 missing narrative |
| **Venue** | No — needs migration | **P0** | "What's the vibe?" — critical for discovery |
| **Release** | Yes (exists, nullable) | **P1** | "About this album" — valuable for deep dives |
| **Label** | Yes (exists, nullable) | **P1** | "Label philosophy and roster" — connects the graph |
| **Festival** | Yes (exists, nullable) | **P1** | "What is this festival?" — already has field |
| **Show** | Yes (exists, nullable) | **P2** | Individual events are ephemeral — lower value |

**Migration:** Add `description TEXT` to `artists` and `venues` tables. Shows, releases, labels, festivals already have the column.

### 4. Trust model: Follow Phase 3 permissions exactly

| User Tier | Can edit descriptions? | Review required? |
|-----------|----------------------|-----------------|
| `new_user` | Yes (suggest) | Yes — enters pending queue |
| `contributor` | Yes (suggest) | Yes — enters pending queue |
| `trusted_contributor` | Yes (direct) | No — edit applies immediately |
| `local_ambassador` | Yes (direct) | No |
| Admin | Yes (direct) | No |

**Submitter trust override:** If you originally submitted the entity, you can edit its description directly regardless of tier (you know the most about what you submitted).

**Edit summary required:** Every description edit must include a summary ("Added band history", "Fixed venue hours", "Updated bio after name change"). This is mandatory — matches Gazelle's `Summary` field and Phase 3's design principle.

### 5. Edit conflict resolution: Last-write-wins + revision history

**Decision:** No locking, no merge conflicts. Last save wins.

**Why:** At PH's current scale (dozens of contributors, not thousands), edit conflicts are extremely rare. Gazelle used the same approach successfully for years. The revision history provides a safety net — any overwritten content is recoverable via rollback.

**Future consideration:** If conflict frequency increases (100+ active contributors editing the same entities), add optimistic locking with a `revision_number` check. Not needed now.

### 6. Integration with existing systems

**Revision history:** Description changes are tracked as a `FieldChange` in the existing RevisionService:
```json
{"field": "description", "old_value": "Previously...", "new_value": "Updated..."}
```
No new revision infrastructure needed.

**Audit log:** Description edits logged via existing fire-and-forget audit log (action: `edit_artist`, `edit_venue`, etc.).

**Notification filters:** Followers of an entity can be notified when its description changes (same trigger as any entity edit).

**Dig Deeper integration (PSY-208):** "Artists with no description" becomes a quality gap category on the /contribute page. "This artist has no description — be the first to add one!" prompt on detail pages.

## Implementation Plan

### Migration (1 file)

```sql
-- 000056_add_entity_descriptions.up.sql
ALTER TABLE artists ADD COLUMN description TEXT;
ALTER TABLE venues ADD COLUMN description TEXT;
-- shows, releases, labels, festivals already have description
```

### Backend changes (minimal)

1. **Models:** Add `Description *string` to Artist and Venue GORM structs (json tag: `"description,omitempty"`)
2. **Handlers:** Artist and venue update handlers already accept field maps — description is just another field
3. **API responses:** Include `description` in artist/venue detail responses (already included for release/label/festival)
4. **Validation:** Max 5,000 chars check in handler Resolve method
5. **No new endpoints needed** — uses existing `PUT /artists/{id}`, `PUT /venues/{id}`, etc.

### Frontend changes

1. **Detail pages:** Add description section below header on artist/venue/release/label/festival detail pages
   - Rendered as Markdown
   - "Edit" button for authorized users (trust-tier-gated)
   - Empty state: "No description yet — add one?" with CTA

2. **Edit UI:** Inline edit mode (not separate page)
   - Textarea with Markdown preview toggle
   - Edit summary field (required)
   - "Save" / "Cancel" buttons
   - For pending-queue users: "Submit for review" instead of "Save"

3. **Revision history:** Existing `RevisionHistory` component already shows field-level diffs — description diffs will appear automatically once RevisionService is wired (PSY-124)

4. **Attribution:** "Last edited by @username, 3 days ago" below description

## What this does NOT include

- **Comments/discussion on entities** — that's a separate feature (show field notes, Phase 3+)
- **Multiple description sections** — one description per entity is sufficient (unlike profile sections)
- **Rich text editor** — Markdown textarea is enough for v1
- **Description templates** — no pre-filled templates ("paste your artist bio here")
- **AI-generated descriptions** — could be a future enhancement but not in scope

## Gazelle reference

- `~/dev/Gazelle` used `wiki_artists` / `wiki_torrents` tables with full revision rows per edit
- Mandatory `Summary` field on every edit
- Permission: `site_edit_wiki` (roughly = PH's `trusted_contributor`)
- Simple content model: one `Body` text field per entity
- No merge conflicts — last write wins, revision history is safety net

## Relationship to other tickets

- **PSY-124** (revision wiring) — must merge first for description diffs to appear in revision history
- **PSY-125** (generic pending edits) — descriptions for `new_user`/`contributor` go through this queue
- **PSY-208** (Dig Deeper) — "artists without descriptions" becomes a quality gap category
- **PSY-210** (contextual prompts) — "Add a description" prompt on detail pages
