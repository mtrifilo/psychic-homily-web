# Curated List Prior Art: Cross-Platform Analysis

> Benchmark reference for Psychic Homily's Collections feature. Analyzes curated list implementations across six platforms to extract patterns, anti-patterns, and concrete UX decisions.

## 1. What Curated Lists Accomplish

Curated lists serve four distinct roles in community platforms, and the strongest implementations nail all four:

1. **Discovery engine** -- Lists are rabbit holes. A user arrives for one item and leaves having found ten. On What.cd, collages like "Intro to Free Jazz" or "Bands with a Male and Female Singer" were described as "indispensable sources of musical discovery." The list is the context that makes an individual item meaningful.

2. **Identity expression** -- A curated list is a taste statement. Letterboxd profiles are defined by lists more than reviews. Bandcamp fan collections signal "I put money behind this." RYM lists with 2,000-word per-item annotations are intellectual identity artifacts. The list says "this is who I am and what I know."

3. **Community knowledge capture** -- The best What.cd collages were collaborative ("Built by N users"), evolving over years as members added discoveries. A "Creation Records 1984-1999" collage isn't one person's opinion -- it's institutional knowledge. Lists are the vessel for expertise that outlives individual contributors.

4. **Social glue** -- Subscribing to a collage on What.cd, liking a list on Letterboxd, or browsing a Bandcamp fan's collection creates lightweight social bonds. You don't need to chat with someone to feel connected through shared taste. Lists are passive social infrastructure.

Platforms that treat lists as a secondary feature (Spotify, early Discogs) get vestigial engagement. Platforms that make lists a first-class citizen (Letterboxd, What.cd, RYM) build community around them.

## 2. Per-System Breakdown

### What.cd / Gazelle -- "Collages"

**Key features:**
- Seven category types: Theme, Genre Introduction, Discography, Label, Staff Picks, Charts, Personal. Fact-based categories (Charts, Label, Discography) enforced objectivity; opinion categories (Personal, Theme, Genre Introduction) allowed subjectivity.
- Subscription model: users subscribed to collages and received notifications when new items were added. `users_collage_subs` tracked `LastVisit` for unread detection. Collages were living, evolving collections.
- Bookmarking: separate from subscription. Users could bookmark a collage without subscribing to updates.
- Comments: full comment threads on each collage, with subscription notifications.
- Collaboration: collages were open to contribution. Multiple users could add torrent groups. The best collages listed contributor counts.
- Personal collage limits tied to user rank: Power Users got 1, Elites got 2, up to 5 for Elite TM+. Donors got one extra. This made personal collages a status marker.
- Minimum 3 items required (except Personal, Label, Staff Picks).
- Duplicate prevention: only one collage allowed per genre or theme. Community self-policed overlap.
- Album artwork displayed as a visual grid on collage pages.
- Each album in a collage linked back to its full torrent group page, showing which other collages it appeared in -- creating a bidirectional discovery web.

**UX strengths:**
- Subscription + notification made collages feel alive. You weren't just bookmarking a static list -- you were joining an ongoing conversation about a topic.
- Category system provided gentle structure without being restrictive. "Theme" was broad enough for almost anything creative.
- Collaboration was the default, not an opt-in feature. This lowered the ego barrier ("this isn't MY list, it's OUR list") and increased quality through collective knowledge.
- Rank-gated personal collages created aspiration. Users wanted to earn more collage slots.

**UX weaknesses:**
- No per-item notes. You could describe the collage, but couldn't explain why a specific album was included. This was a major gap -- the "why" is often more valuable than the "what."
- No ordering control beyond manual sequencing. No ranked/unranked toggle.
- No voting on collages themselves (only on individual torrent groups within them).
- Creation was somewhat buried -- required navigating to a dedicated collage section rather than adding items contextually from entity pages.

**Signature move:** Subscriptions with new-addition notifications. This single feature turned static lists into living community artifacts.

### Discogs -- "Lists"

**Key features:**
- Lists can contain artists, releases, labels, or other lists (meta-lists). This heterogeneous entity support is unusual and powerful.
- Per-item descriptions: each entry in a list can have its own description explaining why it's included.
- Public/private toggle: lists are created private by default, switched to public manually. This reduces the "publish anxiety" that kills list creation.
- Favorites: users can mark lists as favorites, surfacing popular lists.
- Browse page at discogs.com/lists for community discovery.
- No collaboration model -- lists are single-author.

**UX strengths:**
- Mixed entity types in a single list. "My Favorite Label Runs" could contain labels, releases from those labels, and the artists involved -- all in one list.
- Private-by-default reduces friction. Start messy, polish later, publish when ready.
- Per-item descriptions enable the "why" that What.cd collages lacked.

**UX weaknesses:**
- Discovery is weak. The browse page has minimal filtering, no trending algorithm, and no social signals beyond favorites count.
- No notifications or subscriptions. Lists are static snapshots.
- No collaboration. Lists feel like personal filing cabinets rather than community resources.
- Adding items requires navigating to the entity page, clicking "Add to List," and selecting the list. No bulk add, no search-and-add from within the list editor.

**Signature move:** Mixed entity types in a single list. The ability to put an artist, their label, and three key releases in one list creates narrative flexibility that single-entity-type systems lack.

### Bandcamp -- "Fan Collections"

**Key features:**
- Collections are passive -- they consist of music you've purchased plus your wishlist. No active curation beyond buying and wishlisting.
- Fan profiles display collections as visual grids of album art.
- "Supported by" sections on releases show fans who purchased, linking to their profiles. This creates a discovery chain: find a release you like, click a fan who bought it, browse their collection.
- Following: fans follow artists and other fans. The music feed shows activity from followed accounts.
- Bandcamp Clubs (2025): subscription-based curated discovery by "trusted experts," inspired by old-school record clubs.
- Playlists (2025 beta): fans can create playlists from purchased tracks only. Includes custom titles, descriptions, and images.

**UX strengths:**
- The purchase-as-curation model carries inherent credibility. If someone owns 500 albums, their collection is a more honest taste statement than a list they spent 5 minutes assembling.
- Fan-to-fan discovery through "Supported by" is elegant and organic. No algorithm needed.
- The new playlist feature enforces ownership, making each playlist a curated showcase of financial support for artists.

**UX weaknesses:**
- No active curation until the 2025 playlist beta. For over a decade, fans couldn't organize their collections beyond purchase order.
- Playlists limited to purchased tracks only. Can't include wishlist items or items you haven't bought, which limits the discovery use case (you can't recommend what you don't own).
- No collaboration. No commenting on collections. No subscribing to other fans' future purchases.
- No per-item notes on collections (only on the new playlists, which have descriptions).

**Signature move:** Purchase-as-curation. The financial commitment filter produces collections with inherent credibility that free lists can't match.

### Letterboxd -- "Lists" (Gold Standard)

**Key features:**
- Per-entry notes: each film in a list can have a rich-text note (supports LBML formatting with bold, italic, links, blockquotes). This is the critical feature that elevates lists from catalogs to essays.
- Ranked vs. unranked toggle. Ranked lists display numbered positions. Position editing: click a number, type a new position, film moves instantly with automatic renumbering.
- Tags on lists for discoverability. Adding descriptive tags dramatically increases list visibility in browse/search.
- Likes on lists: lightweight social signal. Popular lists surface in browse views.
- Comments on lists: full discussion threads.
- Cloning (Pro feature): one-click copy of any public list to your account, preserving films and tags with a reference back to the original. Fork-and-modify pattern borrowed from Git.
- "Add to Lists" from any film page: contextual button lets you add to multiple lists simultaneously (mobile improvement: multi-select lists in one action).
- Discovery surfaces: Popular lists, Recent lists, Lists by tag, Lists containing a specific film. Featured lists curated by Letterboxd staff.
- List descriptions with Markdown formatting for the curator's narrative framing.
- Visual presentation: poster grid layout makes browsing lists feel like browsing a video store shelf.
- No limit on number of lists per user.

**UX strengths:**
- Per-entry notes transform lists from "what" to "why." A ranked list of 100 horror films with a sentence per entry is fundamentally different from 100 titles in order. The notes are where taste becomes transmissible knowledge.
- Dead-simple creation: start a list, search for films, add them, optionally add notes, save. The barrier from "I have an idea for a list" to "the list exists" is under 60 seconds.
- Contextual "Add to List" from film pages means you build lists as you browse, not in a separate creation mode. This is crucial -- the best time to add something to a list is when you're looking at it.
- Cloning enables remix culture. "I started from [person]'s horror list and added my own favorites" creates social chains of curation.
- Poster-grid visual layout makes lists browsable and beautiful without extra work from the creator.

**UX weaknesses:**
- List quality varies wildly. "Overly broad and irrelevant lists flood the list sections," making discovery noisy. No minimum quality bar.
- No collaboration model. Lists are single-author (cloning is the closest to collaboration, but it's fork, not merge).
- Bulk operations are limited. Adding 50 films to a list one by one is tedious.
- No subscription/notification for list updates. If a list you like gets new entries, you won't know.

**Signature move:** Per-entry notes. This single feature is what makes Letterboxd lists the gold standard. It turns curation from selection into annotation, and annotation is where knowledge lives.

### Rate Your Music (RYM) -- "Lists"

**Key features:**
- 819,000+ user-created lists as of March 2025. Massive corpus covering every imaginable angle.
- List classifiers: categorization system including Themed Lists, Poll Results, Guides, Music Genres, Songs, Lists of Lists. Enables structured browsing.
- Per-item descriptions: users can write as much or as little as they want per entry. Some lists have essay-length annotations per album.
- Comments on lists, with "suggest an addition" mechanism for community input.
- Voting on lists (though the mechanism is less prominent than on reviews/ratings).
- Chart weight: users with more reviews and ratings have more influence on aggregate charts, creating a meritocratic quality signal.
- Automatic recommendations based on user ratings and catalog, with social filtering ("recommendations from people you follow").
- "Sonemic Selects" editorial layer for staff-curated front-page features.

**UX strengths:**
- Depth of annotation. The most dedicated RYM list creators write genuine criticism per entry, turning lists into structured essays. A "Best of Krautrock" list with 50 entries and 200-word descriptions per album is a reference document.
- The classifier system enables browsing by purpose (guides vs. polls vs. themed lists), which helps users find the right kind of list for their mood.
- The sheer volume creates a long tail of niche expertise. Obscure micro-genres that would never get Letterboxd-style attention have dedicated RYM lists.

**UX weaknesses:**
- Catastrophic UX. RYM's interface is described as "death by 1000 cuts" -- inconsistent navigation, poor visual hierarchy, cluttered layouts. Feature richness undermined by presentation and discoverability failures.
- Social features are buried. The friends system, described as "one of the coolest features," is small and obscure. Lists are hard to find through social channels.
- New user onboarding is punishing. The depth that rewards power users actively repels newcomers.
- Visual design from 2002. No poster grids, no modern layout. Text-heavy presentation makes browsing lists a chore.

**Signature move:** Depth of per-item annotation. RYM lists approach music criticism in structure, creating reference documents that transcend casual curation.

### Spotify -- "Playlists" (Comparison Point)

**Key features:**
- 9 billion user-created playlists. Massive scale.
- Collaborative playlists: multiple users can add tracks.
- Algorithmic playlists (Discover Weekly, Daily Mix, AI DJ) generated from listening data.
- Editorial playlists curated by Spotify's internal team (RapCaviar, etc.).
- Playlist descriptions and images.
- Following playlists from other users.

**Why Spotify playlists fail at What.cd-style curation:**

1. **No per-item context.** A playlist is a sequence of tracks with no explanation of why each track is there. The curator's knowledge is invisible. Compare to a What.cd Genre Introduction collage where the description explains the genre's history and each album's role in it.

2. **Algorithmic dilution.** Spotify's shift toward algorithm-driven playlists has "significant drops in streams from flagship playlists like RapCaviar and Dance Hits." When the platform deprioritizes human curation, the curation features atrophy.

3. **No discovery web.** A Spotify playlist doesn't link to the curator's other playlists, show which other playlists a track appears in, or create any bidirectional navigation. Each playlist is an island.

4. **Playback-optimized, not knowledge-optimized.** Playlists are designed to play in sequence, not to be browsed, annotated, or studied. The UX optimizes for "press play and forget" rather than "explore and discover."

5. **No social signals beyond follower count.** No comments, no likes on specific tracks, no "suggest an addition" mechanism. The playlist creator gets no feedback loop.

6. **Discovery Mode corruption.** Artists can accept lower royalty rates for algorithmic promotion, creating a pay-to-play dynamic that undermines curation integrity.

**What Spotify gets right:** Collaborative playlists (the one feature most curation platforms lack) and frictionless creation (drag-and-drop from search results). These are worth stealing even though the overall curation model is weak.

## 3. Synthesis: What Makes These Features Succeed vs. Fail

Seven dimensions separate thriving curation features from vestigial ones:

### Entry points (Where do you start creating?)
- **Winner:** Letterboxd's "Add to List" button on every film page. You create lists where you discover, not in a separate creation area.
- **Loser:** Discogs requiring navigation to the entity page, then "Add to List." Two extra clicks kill impulse curation.
- **Pattern:** The "add to collection" action must be available everywhere an entity appears -- detail pages, search results, cards, even notification feeds.

### Creation friction (How hard is it to go from idea to published list?)
- **Winner:** Letterboxd -- under 60 seconds from "I have an idea" to "the list exists." Name it, add a few films, save.
- **Loser:** RYM's form-heavy interface with no visual feedback during creation.
- **Pattern:** MVP creation should require only a name and one item. Description, notes, ordering, tags -- all optional enhancements added after creation.

### Item addition (How do you add the 2nd through Nth item?)
- **Winner:** Letterboxd's search-and-add within the list editor + contextual "Add to Lists" from browse.
- **Winner:** Spotify's drag-from-search-results into playlist.
- **Loser:** Systems that require navigating away from the list to find items.
- **Pattern:** Two paths: search within the list editor (for focused curation sessions) AND contextual add from entity pages (for serendipitous additions during browse).

### Per-item notes (Can you explain why?)
- **Winner:** Letterboxd (rich text per entry), RYM (essay-length annotations).
- **Loser:** What.cd (no per-item notes), Spotify (no per-track context).
- **Critical insight:** Per-item notes are the single most important feature separating a catalog from a knowledge artifact. Without them, you have a list of things. With them, you have a curated argument.

### Ordering and structure
- **Winner:** Letterboxd's ranked/unranked toggle with instant reorder.
- **Partial credit:** Discogs supports manual ordering but with clunky UX.
- **Pattern:** Support both ranked (numbered, positional) and unranked (thematic grouping) modes. Let the curator choose.

### Social signals (How does the community respond?)
- **Winner:** What.cd's subscription + notification model (the list is alive).
- **Winner:** Letterboxd's likes + comments + cloning (fork the list, remix it).
- **Loser:** Discogs (favorites only, no notifications, no collaboration).
- **Pattern:** Three tiers of social engagement: passive (like/favorite), active (comment/suggest), generative (clone/fork/collaborate).

### Discovery surfaces (How do lists find their audience?)
- **Winner:** What.cd's bidirectional linking (entity pages show which collages contain them; collages show their items). Every entity is an entry point to every list.
- **Winner:** Letterboxd's Popular/Recent/By-tag/By-film browse facets.
- **Loser:** Spotify (no browse, no search, playlists are islands).
- **Pattern:** Lists must appear on entity detail pages ("This artist appears in N collections"). The entity-to-list backlink is the primary discovery surface.

## 4. Concrete UX Patterns to Steal

### From Letterboxd
1. **Per-entry notes with rich text.** Non-negotiable. Each item in a collection should support a curator note explaining why it's there. Markdown formatting minimum.
2. **Ranked/unranked toggle.** Let creators choose whether their collection has positional meaning.
3. **"Add to Collection" from every entity page.** Button on artist detail, show detail, venue detail, release detail. Multi-select to add to multiple collections at once.
4. **Clone/fork with attribution.** "Start from this collection and make it your own" with a link back to the original. Pro/supporter feature.
5. **Poster-grid visual layout.** Display collection items as visual cards (album art, show flyers, venue photos), not text lists.

### From What.cd / Gazelle
6. **Subscription with new-addition notifications.** "Subscribe to this collection" and get notified when items are added. This makes collections living resources, not static snapshots.
7. **Collaboration by default.** Collections should be open to contribution unless the creator locks them. "Built by N contributors" badge.
8. **Bidirectional entity-collection links.** Every entity detail page shows "Appears in N collections" with links. Every collection item links to the full entity page. The web of connections IS the discovery mechanism.
9. **Rank-gated creation limits.** New users get 1-2 personal collections. Trusted contributors get more. This prevents spam and creates aspiration.

### From Discogs
10. **Mixed entity types.** A single collection can contain artists, shows, venues, releases, labels, and festivals. "The Phoenix DIY Scene" might include venues, artists, shows, and a label -- all in one collection.

### From RYM
11. **List classifiers/categories.** Not rigid types, but optional tags that enable structured browse: Guide, Scene Snapshot, Label History, Theme, Personal Favorites. User-applied, not system-enforced.

### From Spotify
12. **Drag-and-drop reordering.** Visual, instant, satisfying. Essential for ranked collections.
13. **Collaborative editing.** Invite specific users to co-curate a collection. Distinct from open collaboration (What.cd style) -- this is controlled co-authorship.

### From Bandcamp
14. **Credibility through investment.** Collections created by users who have contributed data (added artists, verified shows, written notes) carry more weight. Surface contributor reputation alongside collections to signal credibility.

## 5. Anti-Patterns to Avoid

1. **Category rigidity.** What.cd's category types (Theme, Genre Introduction, etc.) were useful guardrails but also limiting. "African Psychedelic Punk from Labels Active in the 1970s" doesn't fit any category. Use optional classifier tags, not mandatory types.

2. **Creation-page-only workflow.** If users can only add items from a dedicated "edit collection" page, they won't. The contextual "Add to Collection" button on entity pages is mandatory.

3. **No minimum quality bar.** Letterboxd is flooded with low-effort lists ("Movies I've Seen," "Movies with Blue in the Title"). Consider requiring a description and minimum 3 items before a collection appears in public browse (private collections can be any size).

4. **Static snapshots.** Discogs lists and Spotify playlists feel dead because there's no notification when they change. If collections can't be subscribed to, they lose the "living document" quality that made What.cd collages magnetic.

5. **Single-author isolation.** Discogs, Letterboxd, and RYM lists are all single-author. This misses the collective knowledge capture that made What.cd collages powerful. Default to open collaboration with creator moderation.

6. **Invisible curators.** Spotify playlists don't link to curator profiles or other playlists by the same person. Always link to the curator's profile and their other collections. The curator is part of the discovery.

7. **No backlinks on entity pages.** If entity detail pages don't show "Appears in N collections," the discovery web is broken. This is the single highest-value integration point.

8. **Text-only presentation.** RYM's text-heavy list display makes browsing a chore. Use visual entity cards (album art, venue photos, show flyers) to make collections browsable without reading.

## 6. Benchmark Rubric

Use this checklist to evaluate any curated list implementation, including Psychic Homily's Collections:

### Creation & Editing
- [ ] Can create a collection in under 60 seconds (name + 1 item minimum)
- [ ] Can add items from entity detail pages (contextual "Add to Collection")
- [ ] Can add items from within the collection editor (search-and-add)
- [ ] Can add to multiple collections at once from an entity page
- [ ] Supports per-item notes with rich text formatting
- [ ] Supports ranked and unranked modes with toggle
- [ ] Drag-and-drop reordering for ranked collections
- [ ] Collection description/narrative with rich text
- [ ] Optional tags/classifiers for discoverability
- [ ] Private draft mode before publishing

### Entity Support
- [ ] Mixed entity types in a single collection (artists + shows + venues + releases + labels + festivals)
- [ ] Bidirectional links: entity pages show "Appears in N collections"
- [ ] Each item links to its full entity detail page

### Social & Collaboration
- [ ] Subscribe to collections with new-addition notifications
- [ ] Like/favorite collections (lightweight social signal)
- [ ] Comments on collections
- [ ] Open collaboration: other users can suggest or add items
- [ ] Clone/fork with attribution (creates a copy linked to original)
- [ ] Creator profile linked from collection (discover their other collections)
- [ ] "Built by N contributors" attribution

### Discovery
- [ ] Browse: Popular, Recent, By Tag/Classifier
- [ ] Search: full-text search across collection names, descriptions, item notes
- [ ] Entity backlinks: clicking "Appears in N collections" from any entity page
- [ ] Curator backlinks: browsing all collections by a specific user
- [ ] Featured/editorial collections (staff picks, seasonal highlights)

### Quality & Moderation
- [ ] Minimum item count for public visibility (e.g., 3 items)
- [ ] Description required for public collections
- [ ] Spam/abuse reporting on collections
- [ ] Creator can moderate suggestions/additions
- [ ] Rank-gated creation limits (more collections for higher-trust users)

### Visual & UX
- [ ] Visual card/grid layout using entity imagery
- [ ] Responsive design (works on mobile)
- [ ] Item count and contributor count visible in browse views
- [ ] Last-updated timestamp visible
- [ ] Smooth transitions for reordering and adding items
