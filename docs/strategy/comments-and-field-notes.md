# Comments & Field Notes — Design Doc

> **STATUS: SHIPPED (April 2026).** Waves 1-5 complete: schema + CRUD (PSY-285), handlers (PSY-286), voting + Wilson score (PSY-287), subscriptions + auto-subscribe (PSY-288), frontend module + entity integration (PSY-290/291), moderation with trust tiers (PSY-292/293), show field notes with verified attendee (PSY-294/295). Comments live on 7 entity types (artists, venues, shows, releases, labels, festivals, collections). Radio episodes not yet integrated. Remaining: Wave 6 (PSY-296/297 — reply permissions, edit history viewer), notifications (PSY-289).
>
> Design doc for PH's community discussion infrastructure and the specialized "show field notes" feature. Retained as architectural reference.

## Problem

PH has zero discussion infrastructure. Users can't:
- Discuss shows ("the opener stole the show"), releases ("this remaster is the one to buy"), or artists
- Share experiential observations about live shows (field notes — PH's unique contribution)
- Add context to collections or radio episodes
- Subscribe to discussion on entities they follow

This is the **biggest missing social feature** in PH (per `docs/learnings/ux-gap-analysis.md` Tier 1 #3) and **a prerequisite for show field notes** (Tier 1 #2).

## Goals

1. **Unified comment system** across all entity types (shows, artists, venues, releases, labels, festivals, collections, radio episodes)
2. **Show field notes** as a specialized comment subtype with structured metadata
3. **Reuse existing PH infrastructure** — trust tiers, entity reports, unified moderation queue, attribution, audit log
4. **Ship quality from day one** — moderation, rate limiting, spam prevention, reply controls
5. **Design for mobile** — web-first but API should support iOS/future clients

## Non-goals (v1)

- Nested threading deeper than 3 levels (use "Continue thread" beyond that)
- Rich text editor (Slate/ProseMirror) — use Markdown
- Vote fuzzing (PH scale doesn't warrant it)
- AutoMod DSL (manual moderation + simple keyword filters sufficient at launch)
- Per-entity forums or multi-topic boards (shoutbox failure mode)
- Wiki-like collaborative editing of comments (single-author with edit history)
- Comment counts with precomputed sort-order caches (simple indexes are fine)
- iOS app support (web-first; iOS post-launch)

## Design Principles

From research synthesis (`docs/learnings/community-discussion-patterns.md`):

1. **Polymorphic by entity_type+entity_id** — matches PH's existing tag/bookmark/report patterns (Gazelle)
2. **Bounded-depth nesting** — 3 levels max, "Continue thread" beyond (Reddit, avoiding flat-design pitfalls)
3. **Wilson score "Best" sort** — default, protects quality new comments (Reddit/Evan Miller)
4. **Trust-tiered publishing** — reuse PSY-126 tier gates
5. **Soft delete with tombstones** — `[deleted]` vs `[removed]` semantics (Reddit)
6. **Markdown, not BBCode** — mature libraries, user-familiar, less than Gazelle's 954 lines of BBCode logic
7. **Verified attendee signal for show field notes** — purchase-gate analog (Bandcamp)
8. **Per-author reply controls** — critical pressure valve from day one (Letterboxd)
9. **Field notes as comment subtype** — not a separate table (shared infrastructure for threading/subscriptions/moderation)
10. **Attribution everywhere** — extends PSY-136 pattern
11. **Rate limiting from day one** — Gazelle's lack was a failure mode
12. **Edit history stored but public only to admins by default** — Reddit asterisk pattern, admin diff via PH's existing revision infrastructure

## Schema

### `comments` table

```sql
CREATE TYPE comment_kind AS ENUM ('comment', 'field_note');
CREATE TYPE comment_visibility AS ENUM ('visible', 'hidden_by_user', 'hidden_by_mod', 'pending_review');
CREATE TYPE reply_permission AS ENUM ('everyone', 'followers', 'none');

CREATE TABLE comments (
  id BIGSERIAL PRIMARY KEY,
  kind comment_kind NOT NULL DEFAULT 'comment',

  -- Polymorphic entity reference
  entity_type VARCHAR(50) NOT NULL,  -- show, artist, venue, release, label, festival, collection, radio_episode
  entity_id BIGINT NOT NULL,

  -- Author
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

  -- Threading (bounded-depth nested)
  parent_id BIGINT NULL REFERENCES comments(id) ON DELETE CASCADE,
  root_id BIGINT NULL REFERENCES comments(id) ON DELETE CASCADE,  -- top-level comment ID for fast thread queries
  depth SMALLINT NOT NULL DEFAULT 0,  -- 0 = top-level, 1 = reply, 2 = reply-to-reply, max 2 enforced in service

  -- Content
  body TEXT NOT NULL,  -- Markdown
  body_html TEXT NULL,  -- cached rendered HTML (regenerated on edit)

  -- Field note structured data (NULL for kind='comment')
  -- Contains: {show_artist_id, song_position, sound_quality, crowd_energy, notable_moments, setlist_spoiler}
  structured_data JSONB NULL,

  -- Reply controls (per author, per-comment override)
  reply_permission reply_permission NOT NULL DEFAULT 'everyone',

  -- Visibility state
  visibility comment_visibility NOT NULL DEFAULT 'visible',
  hidden_reason VARCHAR(255) NULL,
  hidden_by_user_id BIGINT NULL REFERENCES users(id),
  hidden_at TIMESTAMPTZ NULL,

  -- Edit tracking (admin-only history via comment_edits)
  edited_at TIMESTAMPTZ NULL,
  edited_by_user_id BIGINT NULL REFERENCES users(id),
  edit_count INT NOT NULL DEFAULT 0,

  -- Vote aggregates (precomputed for Wilson score)
  ups INT NOT NULL DEFAULT 0,
  downs INT NOT NULL DEFAULT 0,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Wilson score lower bound

  -- Timestamps
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_comments_entity ON comments(entity_type, entity_id, visibility, score DESC);
CREATE INDEX idx_comments_thread ON comments(root_id, depth, created_at);
CREATE INDEX idx_comments_user ON comments(user_id, created_at DESC);
CREATE INDEX idx_comments_kind ON comments(kind) WHERE kind = 'field_note';
CREATE INDEX idx_comments_parent ON comments(parent_id) WHERE parent_id IS NOT NULL;
```

**Key decisions:**
- **`kind` discriminator** — one table for both comments and field notes, different rendering and validation in service layer
- **`root_id`** — top-level comment ID for all replies in a thread, enables fast "load whole thread" queries
- **`depth SMALLINT`** — enforced max 2 in service layer (3 total levels: 0, 1, 2)
- **`structured_data JSONB`** — field note metadata (show_artist_id, song_position, etc.). NULL for regular comments.
- **`visibility enum`** — separates user-delete from mod-delete (Reddit `[deleted]` vs `[removed]`)
- **`score DOUBLE PRECISION`** — Wilson score lower bound, precomputed on vote, indexed for "Best" sort
- **`body_html` cached** — regenerated on edit, not on read
- **No FK to entity tables** — polymorphic pattern, validation in service layer

### `comment_edits` table (append-only edit history)

```sql
CREATE TABLE comment_edits (
  id BIGSERIAL PRIMARY KEY,
  comment_id BIGINT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  editor_user_id BIGINT NOT NULL REFERENCES users(id),
  previous_body TEXT NOT NULL,  -- OLD body before this edit
  edit_reason VARCHAR(255) NULL,  -- optional edit summary
  edited_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_comment_edits_comment ON comment_edits(comment_id, edited_at);
```

**Append-only audit log.** Gazelle pattern: every edit appends the OLD body. Admin can walk back through versions. Users see only "edited N times, last by @username 2h ago" — not diffs.

### `comment_votes` table

```sql
CREATE TYPE vote_direction AS SMALLINT;  -- 1 or -1

CREATE TABLE comment_votes (
  comment_id BIGINT NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  direction SMALLINT NOT NULL CHECK (direction IN (-1, 1)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (comment_id, user_id)
);

CREATE INDEX idx_comment_votes_user ON comment_votes(user_id, created_at DESC);
```

Binary up/down votes. Aggregates (`ups`, `downs`, `score`) stored denormalized on `comments` and updated on write via `ON DUPLICATE KEY UPDATE` pattern.

### `comment_subscriptions` table

```sql
CREATE TABLE comment_subscriptions (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity_type VARCHAR(50) NOT NULL,
  entity_id BIGINT NOT NULL,
  subscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, entity_type, entity_id)
);

CREATE INDEX idx_comment_subscriptions_entity ON comment_subscriptions(entity_type, entity_id);
```

**Per-entity subscription** (not per-comment). Gazelle pattern. Smart defaults:
- Auto-subscribe when user posts a comment (implied interest)
- Manual subscribe button on entity detail pages
- One-click unsubscribe in email notifications

### `comment_last_read` table

```sql
CREATE TABLE comment_last_read (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity_type VARCHAR(50) NOT NULL,
  entity_id BIGINT NOT NULL,
  last_read_comment_id BIGINT NULL REFERENCES comments(id) ON DELETE SET NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, entity_type, entity_id)
);
```

Per-user per-entity "last seen comment ID." Unread count = comments on entity with `id > last_read_comment_id`. Updated on page view of entity's comment section.

### `comment_reports` table

**Decision: reuse existing `entity_reports` with `entity_type='comment'`** rather than separate `comment_reports` table.

Rationale:
- PSY-130/131 entity_reports is already polymorphic
- Unified moderation queue (PSY-132) already aggregates entity reports
- One less table to maintain
- Validation: extend entity_reports to allow `entity_type='comment'` with comment-specific report categories

**Add comment report categories** to existing entity_reports system:
- `spam` — promotional content, repetitive posting
- `harassment` — personal attacks, targeted abuse
- `off_topic` — unrelated to the entity
- `inaccurate` — factually wrong, should be corrected
- `other` — free text required

## Service Architecture

### Package structure

```
backend/internal/services/engagement/
  comment_service.go           # CRUD, threading, validation
  comment_vote_service.go      # Voting, Wilson score
  comment_subscription_service.go  # Subscriptions, unread counts
  comment_notification.go      # Email notifications on new comments/mentions
  comment_moderation.go        # Visibility state machine, rate limiting
```

Place in `engagement/` (alongside bookmarks, attendance, follow, calendar, reminder) since comments are a user engagement primitive.

### Interface (`contracts/comment.go`)

```go
type CommentServiceInterface interface {
  // CRUD
  CreateComment(userID uint, req *CreateCommentRequest) (*CommentResponse, error)
  GetComment(commentID uint) (*CommentResponse, error)
  UpdateComment(userID uint, commentID uint, req *UpdateCommentRequest) (*CommentResponse, error)
  DeleteComment(userID uint, commentID uint, isAdmin bool) error  // soft delete

  // Threading
  ListCommentsForEntity(entityType string, entityID uint, filters CommentListFilters) (*CommentListResponse, error)
  GetThread(rootID uint) ([]*CommentResponse, error)  // full thread including nested replies

  // Field notes
  CreateFieldNote(userID uint, req *CreateFieldNoteRequest) (*CommentResponse, error)
  ListFieldNotesForShow(showID uint) ([]*CommentResponse, error)
}

type CommentVoteServiceInterface interface {
  Vote(userID uint, commentID uint, direction int) error
  Unvote(userID uint, commentID uint) error
  GetUserVote(userID uint, commentID uint) (*int, error)
}

type CommentSubscriptionServiceInterface interface {
  Subscribe(userID uint, entityType string, entityID uint) error
  Unsubscribe(userID uint, entityType string, entityID uint) error
  IsSubscribed(userID uint, entityType string, entityID uint) (bool, error)
  MarkRead(userID uint, entityType string, entityID uint) error
  GetUnreadCount(userID uint, entityType string, entityID uint) (int, error)
}
```

### Moderation flow

```go
// CreateComment visibility logic
func (s *CommentService) computeInitialVisibility(user *User, entityType string, entityID uint) comment_visibility {
  if user.IsAdmin {
    return VisibilityVisible
  }

  // Check rate limit
  recentCount := s.countRecentCommentsBy(user.ID, 60 * time.Second)
  if recentCount >= 1 {
    return VisibilityHiddenByMod // rate-limited, hidden with reason
  }

  // Trust tier gating
  switch user.UserTier {
  case "new_user":
    // Shadow-publish: visible to self, invisible to others until human review
    // OR auto-publish with aggressive spam detection flag
    return VisibilityPendingReview
  case "contributor":
    return VisibilityVisible  // auto-publish but flaggable
  case "trusted_contributor", "local_ambassador":
    return VisibilityVisible
  default:
    return VisibilityPendingReview
  }
}
```

### Rate limiting (per user)

- **Per-entity cooldown**: 60s between comments on same entity by same user
- **Global rate**: 5 comments per hour for `new_user`, 30 per hour for `contributor`+, 100 per hour for trusted
- **Duplicate detection**: comment with identical body to user's recent comment on same entity rejected with "You already posted this"

## API Design

### Public endpoints (optional auth)

```
GET  /entities/{type}/{id}/comments              # list top-level + nested, sort=best|new|top|controversial
GET  /comments/{id}                              # single comment with replies
GET  /comments/{id}/thread                       # full thread including all nested replies
GET  /shows/{id}/field-notes                     # field notes for a show (specialized listing)
GET  /entities/{type}/{id}/comments/unread-count # authenticated: unread count since last read
```

### Protected endpoints (auth required)

```
POST   /entities/{type}/{id}/comments            # create top-level comment
POST   /comments/{id}/replies                    # create reply to comment
POST   /shows/{id}/field-notes                   # create field note
PUT    /comments/{id}                            # edit own comment
DELETE /comments/{id}                            # soft delete own comment
POST   /comments/{id}/vote                       # upvote or downvote
DELETE /comments/{id}/vote                       # remove vote
POST   /entities/{type}/{id}/subscribe           # subscribe to comments on entity
DELETE /entities/{type}/{id}/subscribe           # unsubscribe
POST   /entities/{type}/{id}/mark-read           # mark all comments as read
POST   /comments/{id}/report                     # report comment (creates entity_report)
```

### Admin endpoints

```
POST   /admin/comments/{id}/hide                 # admin hide with reason
POST   /admin/comments/{id}/restore              # admin unhide
DELETE /admin/comments/{id}                      # admin hard delete (rare)
GET    /admin/comments/pending                   # new_user comments awaiting review
POST   /admin/comments/{id}/approve              # approve pending comment
POST   /admin/comments/{id}/reject               # reject pending comment
```

Moderation queue (PSY-132) auto-includes comment reports via entity_reports.

## Frontend Architecture

### Feature module

```
frontend/features/comments/
  components/
    CommentThread.tsx          # list of top-level comments with expand-to-thread
    CommentCard.tsx            # single comment with vote buttons, reply, report
    CommentForm.tsx            # markdown editor with preview
    FieldNoteForm.tsx          # specialized form with structured fields
    FieldNoteCard.tsx          # field note with song position, sound quality badges
    SubscribeButton.tsx        # toggle subscription on entity
    UnreadBadge.tsx            # shows unread count on entity pages
    ReplyPermissionSelect.tsx  # author controls
    index.ts
  hooks/
    useComments.ts             # list + pagination
    useComment.ts              # single comment
    useCreateComment.ts        # create mutation
    useUpdateComment.ts        # edit mutation
    useDeleteComment.ts        # delete mutation
    useVoteComment.ts          # vote with optimistic update
    useSubscribeEntity.ts      # subscribe mutation
    useUnreadCount.ts          # unread count query
    useCreateFieldNote.ts      # specialized field note creation
    useFieldNotes.ts           # field notes for a show
    index.ts
  types.ts
  index.ts
```

### Markdown rendering

- **Library**: `react-markdown` with `remark-gfm` (GitHub-flavored markdown)
- **Sanitization**: `rehype-sanitize` (client) + `bluemonday` (server, for body_html cache)
- **Whitelist**: bold, italic, links, code blocks, lists, blockquotes, headings h3-h6
- **NO**: images, raw HTML, tables (can add later if needed)

### Integration points

**Every entity detail page** (artist, venue, show, release, label, festival, collection, radio episode) gets a `<CommentThread entityType={...} entityId={...} />` section below existing content.

**Show detail page additionally** gets `<FieldNotesSection showId={...} />` that overlays on the show_artists list (showing notes anchored to specific performers/songs) plus a "Show Field Note" CTA for attendees.

**Sidebar** (user-only section) gets a "Discussions" entry showing subscribed threads with unread counts.

**Cmd+K** gets comment-related commands: "Subscribe to discussions on [current entity]", "Mark all as read."

## Moderation Model Integration

### Trust tiers (PSY-126)

| Tier | Create | Publish timing | Edit own | Delete own | Vote | Report | Notes |
|------|--------|---------------|----------|-----------|------|--------|-------|
| `new_user` | Rate-limited | Pending review | Yes (1h window) | Yes | Yes | Yes | Shadow-published or queued |
| `contributor` | Rate-limited | Auto-publish | Yes (24h window) | Yes | Yes | Yes | Flagged if 2+ reports |
| `trusted_contributor` | Rate-limited | Auto-publish | Unlimited | Yes | Yes | Yes | Flagged if 5+ reports |
| `local_ambassador` | Light rate limit | Auto-publish | Unlimited | Yes | Yes | Yes | Report threshold ignored |
| `admin` | No rate limit | Auto-publish | Any comment | Any comment | Yes | N/A | Full moderation power |

### Entity reports integration (PSY-130/131)

Extend `entity_reports` to support `entity_type='comment'`. Add report categories: `spam`, `harassment`, `off_topic`, `inaccurate`, `other`.

On report:
- 1st report: notify admin via Discord (fire-and-forget, existing pattern)
- 3+ reports from trusted users: auto-hide comment pending review (set `visibility = 'hidden_by_mod'`, `hidden_reason = 'multiple reports'`)
- Admin resolves via unified moderation queue (PSY-132)

### Unified moderation queue (PSY-132)

Comments appear in the queue as a third item type alongside pending edits and entity reports:
- **Comment reports** (via entity_reports with `entity_type='comment'`)
- **Pending comments** (from `new_user` tier) — separate query in the queue UI

Update `ModerationQueue.tsx` component to handle the new types with appropriate cards.

### Audit log (PSY-41)

All comment actions logged as fire-and-forget:
- `create_comment`, `edit_comment`, `delete_comment` (user or admin)
- `hide_comment`, `restore_comment` (admin)
- `vote_comment`, `unvote_comment` (user)
- `subscribe_entity`, `unsubscribe_entity`
- `report_comment`, `resolve_comment_report`, `dismiss_comment_report`

### Notification integration (PSY-106/107)

Comment subscriptions are a **separate system** from notification filters, not integrated into `notification_filters` table. Rationale:
- Different semantics: filters match on criteria (genre, city), subscriptions match on entity identity
- Simpler implementation: direct query on `comment_subscriptions` table when comment created
- Future: could unify under a "subscriptions" abstraction if patterns converge

**New comment notification flow:**
1. Comment created on entity X
2. `commentNotificationService.NotifySubscribers(entityType, entityID, commentID)` fires (fire-and-forget)
3. Query subscribers: `SELECT user_id FROM comment_subscriptions WHERE entity_type = ? AND entity_id = ?`
4. For each subscriber: check notification preferences, queue email via existing EmailService
5. Record in `notification_log` for dedup (reuse existing table from PSY-106/107)

**Mention notifications:**
1. On comment create, parse body for `@username` patterns
2. For each mention, check if user exists AND has `notify_on_mention=true`
3. Insert into `notification_log` and send email/in-app notification

## Field Notes Specialization

### What makes field notes different from comments

- **Specialized creation form** with structured fields in addition to freeform body
- **Anchoring to show_artists and song position** via `structured_data` JSONB
- **"Verified Attendee" badge** if author had `user_bookmarks.action='going'` on this show before event date
- **Post-event only** — can't create field notes for shows in the future
- **Displayed differently** — field notes section on show pages, optionally overlaid on setlist timeline

### `structured_data` JSONB schema for field notes

```typescript
{
  "show_artist_id": 42,           // which performer the note is about (optional for show-wide notes)
  "song_position": 7,             // position in setlist (optional, NULL for between-song or overall)
  "sound_quality": 4,             // 1-5 scale (optional)
  "crowd_energy": 5,              // 1-5 scale (optional)
  "notable_moments": "Played 3 new songs from the upcoming album",  // structured highlight (optional)
  "setlist_spoiler": true,        // flag if note reveals unannounced setlist items
  "is_verified_attendee": true    // computed at creation time, cached
}
```

All fields optional except the body (from the main `comments` table). Field notes can be:
- **Show-wide**: no show_artist_id, no song_position, body is general observation
- **Artist-anchored**: show_artist_id set, no song_position, body is about one performer's whole set
- **Moment-anchored**: both show_artist_id and song_position set, body is about a specific song

### Soft anchoring fallback

If a setlist is edited after a field note is created and `song_position=7` no longer exists (or is a different song), the UI falls back gracefully:
- Display note under `show_artist_id` instead (artist's whole set)
- Show warning: "This note was anchored to song position 7, which has changed. Current setlist shows X."

### Attendee verification

When creating a field note:
```go
func (s *CommentService) computeAttendeeVerification(userID uint, showID uint) bool {
  show := s.showRepo.Get(showID)
  if show.EventDate.After(time.Now()) {
    return false  // show hasn't happened yet
  }

  bookmark := s.bookmarkRepo.Find(userID, "show", showID, "going")
  if bookmark == nil {
    return false
  }

  // Must have marked "going" BEFORE show date
  return bookmark.CreatedAt.Before(show.EventDate)
}
```

Result cached in `structured_data.is_verified_attendee`. Displayed as a badge next to author name on field notes.

## Performance & Scaling

### Read performance
- **Index on `(entity_type, entity_id, visibility, score DESC)`** — for default "Best" sort
- **Index on `(root_id, depth, created_at)`** — for full-thread queries
- **No precomputed sort-order caches** — SELECT + ORDER BY is sufficient at PH scale for years
- **body_html cached** on the comment row, regenerated only on edit

### Write performance
- Vote aggregates updated inline: `UPDATE comments SET ups=..., downs=..., score=... WHERE id=?`
- Wilson score computed in Go (reuse existing utility from tag voting)
- Audit log + Discord notification are fire-and-forget (no impact on request)

### Scalability
At current PH scale (single-digit k users), single-table polymorphic pattern handles millions of comments without issue. If PH eventually needs to scale:
- Partition `comments` by `entity_type`
- Add materialized views for hot entities
- Move to CQRS with separate read/write paths

These are Year 3+ concerns.

## Build Order (Wave Structure)

### Wave 1: Foundation (3-4 tickets)

**Blocks all other waves.** Minimum viable comments.

1. **Backend: Comments schema + basic CRUD**
   - Migrations: `comments`, `comment_edits`, `comment_votes`
   - GORM models
   - `CommentService` with: create (auto-publish for trusted+), get, list-for-entity, update (own), delete (soft, own/admin)
   - Markdown rendering server-side (goldmark + bluemonday)
   - Polymorphic `(entity_type, entity_id)` validation
   - 20+ tests (service integration + nil-db)

2. **Backend: Comment handlers + API routes**
   - Public endpoints: list, get, thread
   - Protected endpoints: create, update, delete
   - Huma request/response types
   - Register routes in `routes/routes.go`
   - 15+ handler tests (mock-based)

3. **Backend: Comment voting + Wilson score**
   - `CommentVoteService`
   - Vote/unvote with aggregate updates
   - Wilson score computation (reuse from tags)
   - Sort by "best" (default), "new", "top", "controversial"
   - 15+ tests

### Wave 2: Subscriptions & Notifications (2-3 tickets)

4. **Backend: Comment subscriptions + unread tracking**
   - Migrations: `comment_subscriptions`, `comment_last_read`
   - Subscribe/unsubscribe API
   - Unread count endpoint
   - Auto-subscribe on first comment by user (smart default)
   - 10+ tests

5. **Backend: Comment notification integration**
   - `CommentNotificationService` — fire-and-forget email to subscribers
   - Integration with existing EmailService (PSY-106/107 email templates)
   - `notification_log` dedup
   - Mention parsing (`@username`)
   - 10+ tests

### Wave 3: Frontend Foundation (2-3 tickets)

6. **Frontend: Comment feature module**
   - `features/comments/` with components, hooks, types
   - `CommentThread`, `CommentCard`, `CommentForm`
   - Markdown rendering with `react-markdown`
   - Vote UI with optimistic updates
   - Subscribe button
   - Unread badge
   - Feature module tests

7. **Frontend: Entity integration (all 8 entity types)**
   - Add `<CommentThread />` to: ArtistDetail, VenueDetail, ShowDetail, ReleaseDetail, LabelDetail, FestivalDetail, CollectionDetail, RadioEpisodeDetail
   - Cmd+K: subscribe command
   - Sidebar: "Discussions" entry with unread counts
   - Test updates

### Wave 4: Moderation (2 tickets)

8. **Backend: Comment moderation integration**
   - Extend `entity_reports` to support `entity_type='comment'` (new category constants)
   - Trust-tier publishing logic (new_user → pending, contributor+ → auto-publish)
   - Rate limiting middleware
   - Admin endpoints: hide, restore, approve pending
   - Auto-hide on 3+ reports from trusted users
   - Integration with unified moderation queue (PSY-132)
   - 15+ tests

9. **Frontend: Admin comment moderation UI**
   - Add "Comment reports" filter to moderation queue
   - Pending comments admin tab
   - Hide/restore actions
   - Report categories in report dialog

### Wave 5: Field Notes Specialization (2 tickets)

10. **Backend: Field notes as comment subtype**
    - `kind` enum + `structured_data` JSONB validation for field notes
    - `CreateFieldNote` service method with attendee verification
    - Post-event gating (show.event_date < now)
    - List field notes for show endpoint
    - 15+ tests

11. **Frontend: Field notes UI**
    - `FieldNoteForm` with structured fields (show_artist_id picker, song_position, sound_quality, crowd_energy)
    - `FieldNoteCard` with verified attendee badge
    - Show detail page integration — separate "Field Notes" section
    - Setlist spoiler flag
    - Soft-anchor fallback rendering

### Wave 6: Polish (1-2 tickets)

12. **Per-author reply permission controls**
    - `reply_permission` enum in model
    - Author-side UI to change reply permission per comment
    - Frontend gating (hide reply button if not permitted)

13. **Comment edit history (admin-only)**
    - Admin diff viewer for `comment_edits` table
    - Walkback UI per Gazelle pattern

## Design Decisions Summary

| Decision | Choice | Source |
|----------|--------|--------|
| Threading model | Bounded nested (3 levels) + "Continue thread" | Reddit minus depth |
| Markup format | Markdown (goldmark + react-markdown) | Gazelle warning |
| Default sort | Wilson score "Best" | Reddit |
| Anti-spam gate | Trust tiers + rate limiting + verified attendee for shows | PSY-126 + Bandcamp |
| Field notes as subtype | `kind` enum + `structured_data` JSONB on same table | Reduces duplication |
| Deletion model | Soft delete with `[deleted]`/`[removed]` semantics | Reddit |
| Edit history | Stored in append-only `comment_edits`, admin-only diff | Gazelle + Reddit |
| Notifications | Separate `comment_subscriptions` table (not via notification_filters) | Simpler semantics |
| Reply permissions | Per-author per-comment: everyone / followers / none | Letterboxd |
| Reactions | None in v1 (up/down votes only) | Simplicity |
| Comment reports | Reuse `entity_reports` with `entity_type='comment'` | PSY-130/131 |
| Moderation queue integration | Yes, comments appear alongside pending edits + entity reports | PSY-132 |
| Voting system | Binary up/down with Wilson score | Gazelle/Reddit |
| Scope at launch | All 8 entity types (artist/venue/show/release/label/festival/collection/radio_episode) | Comprehensive |
| iOS support | Web-first, iOS post-launch | Simpler v1 |

## Open Questions (deferred)

- **Should mentions count toward contributor stats?** Probably not — could be abused.
- **How does comment-voting relate to tier promotion?** Comment karma as a separate signal? Not in v1.
- **Should we allow pinning top comments per entity?** Nice feature for future, not v1.
- **What about comments on user profiles (guestbook)?** Defer — different semantics, different spam vectors.
- **Should field notes have their own URL?** `/shows/{slug}/notes/{id}` — yes, for shareability.

## Reference Mapping

| PH Design Choice | Prior Art Inspiration |
|-----------------|---------------------|
| Polymorphic `(entity_type, entity_id)` | Gazelle `(Page, PageID)` |
| 3-level bounded nesting | Reddit (capped from their 10-level default) |
| Wilson score "Best" sort | Reddit (Evan Miller algorithm) |
| Append-only edit history | Gazelle `comments_edits` |
| `[deleted]` vs `[removed]` | Reddit |
| Markdown over BBCode | Modern convention, avoid Gazelle complexity |
| Verified attendee badge | Bandcamp purchase gate analog |
| Per-author reply permissions | Letterboxd |
| Field notes structured_data | Genius annotations (structural anchoring) |
| Setlist spoiler flag | Letterboxd mandatory spoiler flag |
| Trust-tier publishing gates | Gazelle warning system + PSY-126 tiers |
| Report-based auto-hide at threshold | Reddit AutoMod-lite |
| Per-entity subscriptions (not per-comment) | Gazelle `users_subscriptions_comments` |
| Mandatory source on setlist edits | Setlist.fm editorial pattern |
| Colored tier badges on author | Setlist.fm role badges |
| Staff rotation as curation override | RYM hybrid curation model |
| Private attendee notes escape hatch | Setlist.fm private notes |

## Ticket Breakdown

After this design is approved, create the following tickets in the **Community Discussion & Field Notes** Linear project:

1. **Backend: Comments schema + CRUD service** (Wave 1)
2. **Backend: Comment handlers + API routes** (Wave 1)
3. **Backend: Comment voting + Wilson score** (Wave 1)
4. **Backend: Comment subscriptions + unread tracking** (Wave 2)
5. **Backend: Comment notification integration** (Wave 2)
6. **Frontend: Comment feature module** (Wave 3)
7. **Frontend: Entity integration for comments** (Wave 3)
8. **Backend: Comment moderation integration** (Wave 4)
9. **Frontend: Admin comment moderation UI** (Wave 4)
10. **Backend: Field notes as comment subtype** (Wave 5)
11. **Frontend: Field notes UI** (Wave 5)
12. **Per-author reply permission controls** (Wave 6)
13. **Admin comment edit history viewer** (Wave 6)

Each ticket should be small enough to ship in 1-2 days. Waves have dependencies (1 → 2 → 3 → 4/5 can run in parallel → 6).
