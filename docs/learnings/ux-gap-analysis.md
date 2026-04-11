# UX Gap Analysis — April 2026

> **PARTIALLY STALE (as of April 11, 2026).** Items #1 (comments) and #2 (field notes) from the Tier 1 priority list are now SHIPPED. Collections UX overhaul also shipped. Re-read with that context — many "PH today: not built" claims are now outdated.
>
> Comparison of PH's current implementation against proven UX patterns from What.cd/Gazelle, Discogs, Bandcamp, RYM, Setlist.fm, and fandom communities.

## Reference Sources

- **What.cd / Gazelle**: `docs/learnings/gazelle-patterns.md`, `gazelle-user-profiles.md`, `whatcd-user-insights.md`
- **Discogs**: Master release → versions model, submission guidelines, credit roles, physical format detail
- **Bandcamp**: Direct artist support, fan collections as social proof, editorial curation (Bandcamp Daily)
- **RYM (Rate Your Music)**: Community ratings/reviews, user lists, genre classification depth
- **Setlist.fm**: Structured setlist data, attendee contributions

## Tier 1 — Core Moat Gaps

Features whose absence prevents PH from being *the* authoritative source of truth.

### 1. Edition-Level Release Provenance

**What it is:** A release (e.g., "OK Computer") has multiple editions — original 1997 Parlophone CD, 2017 OKNOTOK remaster, Japanese bonus-track pressing, vinyl reissue. Each edition has distinct: label, catalog number, country, format (CD/LP/digital), year, mastering notes, barcode.

**Who does it well:**
- What.cd: The Oxygène screenshot shows 14+ editions with full metadata. Users could answer "which remaster sounds best?" — the core use case.
- Discogs: Master release → versions model. Each version has label, format, country, year, barcode, tracklist variations.
- MusicBrainz: Release group → releases → mediums → tracks hierarchy.

**PH today:** `Release` is a flat entity — one row per release with single `release_type`, `release_year`, `cover_art_url`. No editions, no catalog numbers, no format detail, no pressing variations.

**Impact:** Without this, PH's release pages are thinner than MusicBrainz. Users can't answer "which version should I buy?" — the question that made What.cd indispensable. Radio plays and show setlists link to a single release entity when the *specific edition* matters for provenance.

**Design considerations:**
- Add `release_editions` table: release_id, label_id, catalog_number, country, format, year, mastering_notes, barcode, cover_art_url
- External links (Bandcamp, Spotify, etc.) belong on editions, not releases
- Matching engine (radio plays) should prefer edition-level links when MusicBrainz release ID is available
- Migration path: existing releases become the "master" entity; first edition auto-created from current data

### 2. Show Field Notes

**What it is:** Structured qualitative observations from show attendees. Not star ratings — experiential data. "Sound was incredible from the balcony," "they played 3 unreleased songs," "opened with a 20-minute improv," "the opener stole the show."

**Who does it well:** Nobody does this well. Setlist.fm has setlists but not experiential notes. Concert review blogs exist but aren't structured or community-contributed. This is PH's unique contribution.

**PH today:** Not designed. Listed in Phase 3 roadmap as "design as first entity comment type" but no design doc exists. The `Show` model has no comments, notes, or review infrastructure.

**Impact:** Live show intelligence is PH's differentiator. Without field notes, show pages are calendar listings with artist lineups — the same as Bandsintown. Field notes make PH *the* place to understand what the live experience is actually like.

**Design considerations:**
- Build on top of a polymorphic comment/discussion system (see gap #3)
- Structured fields beyond freeform text: sound_quality (1-5), crowd_energy (1-5), notable_moments (freeform), personal_highlights (freeform)
- Visibility: only after show date has passed (no pre-show speculation)
- Trust-tiered: new users' notes go through moderation; trusted contributors publish immediately
- Attendee verification: stronger signal if user marked "going" beforehand

### 3. Comments / Discussion on Entities

**What it is:** Threaded discussion attached to any entity — shows, releases, artists, collections, radio episodes. The substrate for community knowledge exchange.

**Who does it well:**
- Gazelle: Threaded comments on every torrent, collage, and artist page. Comment subscriptions for notifications.
- Discogs: Reviews on releases. Community discussion on submissions.
- RYM: Reviews and discussion threads on albums and artists.
- Reddit/forums: General discussion, but not entity-attached.

**PH today:** Zero discussion infrastructure. No comments on any entity type. The `features/` modules have no comment components. No `comments` table in the data model.

**Impact:** Community knowledge lives in discussion. Without comments, PH is a catalog, not a community. Users can't discuss "is this the right artist?" on an artist page, can't share opinions on releases, can't discuss show experiences. This is the biggest missing social feature.

**Design considerations:**
- Polymorphic `comments` table: user_id, entity_type, entity_id, parent_id (threading), body (markdown), created_at
- Comment subscriptions: notify users when new comments appear on entities they follow
- Moderation: trust-tiered (new users moderated, trusted contributors auto-publish), report button, admin delete
- Show field notes as a specialized comment type with structured metadata
- Rate limiting and spam prevention

### 4. User Ratings and Reviews

**What it is:** Community-sourced quality signals on releases and shows. Star ratings, short reviews, aggregate scores.

**Who does it well:**
- RYM: 0.5–5.0 star ratings with written reviews. Aggregate scores drive discovery ("top albums of 2025").
- Gazelle: Star ratings on torrents.
- Bandcamp: Implicit ratings via purchases and collections.
- Discogs: 1–5 star ratings with reviews.

**PH today:** Wilson-scored voting on tags and requests. No rating/review system for releases, shows, artists, or any other entity.

**Impact:** Discovery through community taste is unbuilt. Users can browse by tag, by relationship graph, by radio co-occurrence — but not by "what do people think is good?" Ratings power charts, recommendations, and the "wisdom of crowds" discovery path.

**Design considerations:**
- `entity_ratings` table: user_id, entity_type, entity_id, rating (0.5-5.0 scale, half-stars), review_text (optional), created_at
- Aggregate scores: weighted average displayed on entity pages, updated on write
- Release ratings: the highest-value target (maps to RYM/What.cd core pattern)
- Show ratings: post-event only, complementary to field notes
- Charts integration: "highest rated releases this month" alongside existing trending/popular charts
- Wilson score for ranking with low vote counts

## Tier 2 — Identity & Engagement Gaps

Features that deepen contributor attachment and long-term retention.

### 5. "Your Impact" Metrics

**What it is:** Showing contributors the downstream effect of their work. Not just "you made 50 edits" but "artists you added have been viewed 12,000 times this month."

**Who does it well:**
- Gazelle: "Your uploads have been snatched N times" on profiles.
- Wikipedia: Edit count + article view counts for pages you've edited.
- GitHub: Contribution graph + "used by N repositories."

**PH today:** 23 contribution stat types — all input metrics (edits, tags voted, reports filed). Zero output metrics showing the impact of those contributions.

**Impact:** The What.cd user insight research found that contributor identity attachment ("part of me died when it shut down") is the strongest retention signal. Impact metrics make abstract contributions feel tangible.

**Design considerations:**
- Compute periodically (daily): total views on entities the user created/edited, total followers on artists they added, total saves on shows they submitted
- Display on contributor profile alongside existing stats
- "Your artists have been viewed 5,200 times this month" — simple, motivating

### 6. Knowledge Graph Export / Data Survivability

**What it is:** An explicit promise and mechanism for community-contributed data to survive. Downloadable exports in standard formats.

**Who does it well:**
- MusicBrainz: Full database dumps (weekly), JSON-LD API, CC0 licensing.
- Wikipedia: Full dumps, open license.
- Discogs: Monthly data dumps (XML/CSV), CC0 for user-contributed data.

**PH today:** GDPR user data export exists. No knowledge graph export. No licensing model for community-contributed data. No design doc.

**Impact:** The What.cd user insights doc ends with: "The treasure is still out there — we have merely lost the map." PH must explicitly address data survivability to earn trust from power contributors. Without it, contributors who remember What.cd will hold back.

**Design considerations:**
- Export formats: JSON-LD (linked data standard), CSV (spreadsheet-friendly), PostgreSQL dump (developer-friendly)
- Licensing: CC0 for community-contributed facts (artist names, show dates, setlists). CC-BY for editorial content (reviews, field notes).
- Frequency: Weekly automated dumps, on-demand via API
- Scope: All public entity data, relationships, tags, aggregate ratings. Exclude: user accounts, private data, API tokens.

### 7. Ranked Lists

**What it is:** User-created ordered lists. "My top 50 albums of 2025," "Best live shows I've seen," "Essential shoegaze EPs."

**Who does it well:**
- RYM: User lists are a primary discovery feature. "Top albums of [year]" aggregate lists drive traffic.
- Letterboxd: Film lists with rankings, descriptions, social sharing.
- Gazelle: Collages (unranked collections) — PH already has this equivalent.

**PH today:** Collections (crates) exist with full CRUD, items, subscriptions. But items are unordered sets — no position/ranking. No "list" concept distinct from "collection."

**Impact:** Lists are the primary shareable artifact for music communities. "My top 10 of the year" posts drive social media traffic and discovery. An ordered collection is a trivial extension of the existing system.

**Design considerations:**
- Add `position` field to `collection_items` (nullable — unordered collections remain valid)
- UI: drag-and-drop reordering, numbered display for ranked lists
- New collection type flag: `is_ranked` boolean
- Aggregate lists: "Community top albums of 2025" computed from individual user lists (like RYM's aggregate charts)

## Tier 3 — Discovery Depth Gaps

Features that make the knowledge graph richer and more interconnected.

### 8. Structured Setlists

**What it is:** Per-song data for a show's set — song title linked to release, position in set, encore flag, guest performers, cover song attribution.

**Who does it well:**
- Setlist.fm: Core feature. Community-contributed setlists for millions of shows.
- Songkick (defunct): Had setlist data integration.

**PH today:** `Show` model has a `set_list` text field — freeform, unstructured. No linking songs to releases in the knowledge graph. Radio plays already link tracks to artists/releases, but show setlists don't.

**Impact:** Setlists close the loop between live shows and recorded music. "They played 3 songs from their new album" → links show → release → label. Also enables: "how often does this artist play this song live?", "what's their most common opener?", fan-favorite deep cuts.

**Design considerations:**
- `setlist_items` table: show_id, position, song_title, release_id (nullable), is_encore, is_cover, cover_original_artist_id, notes
- Community-contributed: attendees add/edit setlists post-show
- Trust-tiered like other edits
- Integration with radio plays: if a radio play matches a setlist item, cross-link

### 9. Musician Entity / Scene Family Tree

**What it is:** Tracking individual musicians across multiple bands over time. "Dave Grohl: Nirvana (1990-1994) → Foo Fighters (1994-present) → Them Crooked Vultures (2009-2010)."

**Who does it well:**
- MusicBrainz: Full artist → member relationships with date ranges.
- Discogs: Credits at track level include personnel.
- AllMusic: Band member timelines.

**PH today:** `artist_relationships` has a `member_of` type but no date ranges, no individual musician entity. Artists are bands/projects, not people.

**Impact:** Local scene intelligence — "these 3 bands share a drummer" — is a discovery path unique to PH's live show data. Currently unmapped.

**Design considerations:**
- `musicians` table: name, slug, instruments, bio
- `musician_memberships`: musician_id, artist_id, role (guitar/vocals/drums), start_date, end_date, is_active
- Discovery: "Other bands with members of [artist]" sidebar section
- Scene family tree visualization: network graph of musicians across bands

### 10. Promoter Entity

**What it is:** The booking agents and promotion companies that organize shows. Maps the business relationships that determine which artists play which venues.

**PH today:** Not modeled. Shows have venues and artists but no promoter attribution.

**Design considerations:**
- `promoters` table: name, slug, city, state, website, social
- `show_promoters` junction: show_id, promoter_id
- Discovery: "Shows by [promoter]", "Venues where [promoter] books", "Artists [promoter] works with"
- Professional audience: venue managers, booking agents, artists seeking gigs

### 11. Multi-Image Galleries

**What it is:** Multiple community-contributed images per entity — show flyers, venue photos, release artwork variants, festival posters.

**Who does it well:**
- Discogs: 10+ images per release (front, back, inner sleeve, disc, matrix).
- Gazelle: Multiple cover images per torrent group.

**PH today:** Single `cover_art_url`/`logo_url` fields on entities. No gallery infrastructure.

**Design considerations:**
- `entity_images` table: entity_type, entity_id, url, caption, uploaded_by, position, is_primary
- Community-contributed with moderation
- Show flyers are particularly valuable — ephemeral artifacts that PH could preserve

## Tier 4 — Engagement Mechanics

Proven patterns not yet applied.

### 12. Collection "New Since Last Visit"

Gazelle showed unread counts on subscribed collages. PH collections have subscriptions but no "new items since you last viewed" indicator. Cheap engagement driver — a badge count on the sidebar.

### 13. Comment/Discussion Subscriptions

When comments ship (gap #3), users need subscription notifications. Gazelle's polymorphic `users_subscriptions_comments` pattern is documented in learnings but not implemented.

### 14. Artist Opt-Out Mechanism

A "do not list" mechanism for artists who request removal. Important for trust-building as PH grows. Staff-maintained exclusion list.

### 15. Editorial Curation Framework

Bandcamp Daily-style featured content — staff picks, themed collections, artist spotlights. PH has a blog feature but no structured editorial framework for surfacing community-curated content.

## What PH Already Exceeds

Areas where PH is ahead of all reference sites:

- **Community contribution infrastructure**: Trust tiers, pending edits, auto-promotion, entity reports, unified moderation — more sophisticated than any reference
- **Radio integration**: 3 providers, matching engine, co-occurrence → artist similarity — no reference site has this
- **AI-first data pipeline**: Automated venue calendar extraction — unique to PH
- **Show-centric knowledge graph**: Live shows as first-class discovery gateway — no reference does this
- **Tag voting with Wilson score**: Matches Gazelle quality, better than Discogs/RYM

## Recommended Priority Order

1. **Comments/discussion** — prerequisite for #2 and #4, the missing social substrate
2. **Show field notes** — PH's unique contribution, built on comments
3. **Edition-level releases** — the What.cd depth that makes PH authoritative for recorded music
4. **Ratings on releases and shows** — unlock discovery-by-taste
5. **"Your impact" metrics** — cheap, high-ROI contributor retention
6. **Ranked lists** — trivial extension of existing collections, high shareability
7. **Knowledge graph export** — trust-building, data survivability promise
8. **Structured setlists** — close the live→recorded loop
