# Future Product Roadmap

## Overview

Features that drive both user growth and monetization potential. Each feature is evaluated on its ability to:
1. Attract and retain users
2. Create revenue opportunities
3. Build network effects and competitive moats
4. Align with the core mission (local music discovery)

---

## Feature Ideas

### 1. "Who's Going" Social Layer

**Growth driver**: FOMO is the #1 motivator for live music attendance

**How it works**:
- Users can mark "Going" or "Interested" on shows
- See which friends/followed users are attending
- Optional: connect Spotify to find users with similar taste
- Show attendance count on show cards ("23 people interested")

**Monetization unlock**:
- Venues/promoters pay for visibility into attendance predictions
- "X people interested" becomes social proof that sells tickets
- Targeted "your friends are going" push notifications (premium feature for promoters)

**Network effect**: More users → more social signal → more reason to join

**Technical considerations**:
- New `show_attendance` table (user_id, show_id, status: going/interested)
- Privacy controls (public/friends-only/private)
- Follow system for users
- Real-time count updates

---

### 2. Personalized Show Recommendations

**Growth driver**: "Spotify Discover Weekly for local shows"

**How it works**:
- Connect Spotify/Apple Music to analyze listening history
- Match against artists playing locally
- Weekly personalized email: "Shows for you this week"
- In-app "For You" section on homepage

**Monetization unlock**:
- Sponsored slots in recommendation emails
- "Recommended" shows can be paid placements (clearly labeled)
- Premium users get SMS alerts for high-match shows

**Retention hook**: Personalized value keeps users coming back

**Technical considerations**:
- Spotify OAuth already exists - extend to read listening history
- Artist matching algorithm (fuzzy name matching, genre similarity)
- Recommendation scoring system
- Email personalization infrastructure

**Existing foundation**:
- Spotify artist links already in database
- Email infrastructure (Resend) in place
- OAuth patterns established

---

### 3. Artist Claim & Verification System

**Growth driver**: Artists promote their own profiles, bringing their fans

**How it works**:
- Artists claim their profile (verify via Spotify/Bandcamp OAuth)
- Claimed artists can edit their bio, add photos, link socials
- Show "Verified Artist" badge on profile and show listings
- Artists get notified when added to a show

**Monetization unlock**:
- Free tier: claim profile, basic stats (show count, saves)
- Pro tier ($10/mo): detailed analytics, fan demographics, export for booking pitches
- Artists share their profiles → free marketing for platform

**Network effect**: Artists bring fans, fans discover more artists

**Technical considerations**:
- New `artist_claims` table linking users to artists
- Verification flow via OAuth (Spotify artist account, Bandcamp)
- Artist dashboard with edit permissions
- Notification system for show additions

**Existing foundation**:
- Artist model exists with Spotify/Bandcamp URLs
- OAuth infrastructure in place
- Admin verification patterns established (venues)

---

### 4. Post-Show Content (Photos/Reviews)

**Growth driver**: Extend the lifecycle of a show from 1 day to weeks

**How it works**:
- After a show ends, attendees can upload photos
- Short reviews/ratings (1-5 stars + optional text)
- "Best of the night" highlights curated by admins
- Creates archive of local scene history

**Monetization unlock**:
- Venues pay for professional photo integration
- Photographers get portfolio exposure (future: booking marketplace)
- Nostalgia engagement drives repeat visits
- SEO goldmine (long-tail searches for past shows)

**Content moat**: Unique user-generated content competitors can't replicate

**Technical considerations**:
- Image upload and storage (S3/Cloudflare R2)
- Content moderation workflow
- Show state transitions (upcoming → past → archived)
- Review/rating system with spam prevention

**Growth loop**:
- Users upload photos → tag friends → friends join to see/upload → more content

---

### 5. Calendar Sync + Smart Reminders

**Growth driver**: Utility that embeds into daily life

**How it works**:
- One-click add to Google/Apple/Outlook calendar
- ICS feed for all saved shows (auto-updates)
- Smart reminders: "Show tonight at 8pm - doors at 7"
- Day-of notifications with venue directions, parking tips

**Monetization unlock**:
- Reminder notifications can include sponsor message
- "Pre-game at [sponsor bar] before the show"
- Premium: SMS reminders, group coordination features

**Stickiness**: Becomes the system of record for their live music calendar

**Technical considerations**:
- ICS feed generation per user
- Google Calendar API integration for one-click add
- Push notification infrastructure (web push, optional mobile app)
- Reminder scheduling system

**Low effort, high value**: Calendar export is simple but surprisingly sticky

---

### 6. Multi-City Expansion Framework

**Growth driver**: 10x addressable market

**How it works**:
- Start with neighboring scenes: Tucson, Flagstaff, Albuquerque
- Each city has local curators/admins
- Users can follow multiple cities
- Eventually: Denver, Austin, LA, Portland

**Monetization unlock**:
- Each city = new venue/promoter customer base
- Regional sponsors (statewide radio stations, beer brands, tour promoters)
- Franchise model: license platform to local operators

**Scale**: Local depth + geographic breadth

**Technical considerations**:
- City/region as first-class entity in data model
- Multi-city venue and show management
- City-specific admin permissions
- User city preferences and multi-city feeds

**Expansion strategy**:
1. Phoenix (current) - prove the model
2. Tucson - test expansion playbook (2 hours away, smaller scene)
3. Flagstaff/Sedona - tourist + college market
4. Albuquerque - out-of-state test
5. Evaluate: build vs. license vs. partner

---

### 7. Venue Analytics Dashboard

**Growth driver**: Venues promote their listings, becoming platform advocates

**How it works**:
- Venues see: profile views, show saves, ticket clicks, traffic sources
- Demographic breakdown: age ranges, neighborhoods, music preferences
- Competitive intel: "Similar venues are averaging X saves/show"
- Performance trends: "Your Tuesday shows underperform Fridays by 40%"

**Monetization unlock**:
- Free tier: basic view count, last 30 days
- Pro tier ($50/mo): full analytics, demographics, export, historical data
- Venues justify subscription to management with data ROI

**B2B wedge**: Venues have marketing budget, data has clear value proposition

**Technical considerations**:
- Analytics event tracking (already planned with PostHog)
- Aggregation and reporting layer
- Venue dashboard UI
- Data privacy considerations (aggregate only, no individual user data)

**Sales motion**:
- Start with 2-3 friendly venues as design partners
- Build for their specific needs
- Use case studies to expand

---

## The Flywheel

```
More users → More "going" signals → Better recommendations
     ↑                                      ↓
Artists promote ← Venues see value ← Higher attendance
their profiles     in analytics
```

Each feature reinforces the others:
- More users = more social proof = more reason for new users to join
- Artists promoting profiles = free distribution = more users
- Better data = better recommendations = higher engagement = more data
- Venues seeing ROI = more venue investment = better show data = better user experience

---

## Prioritization Matrix

| Feature | Growth Impact | Revenue Potential | Effort | Suggested Phase |
|---------|---------------|-------------------|--------|-----------------|
| Calendar sync + reminders | High | Low | Low | Phase 1 (Now) |
| Artist claim/verification | High | Medium | Medium | Phase 2 (Q2) |
| Who's going | Very High | Medium | Medium | Phase 2 (Q2) |
| Personalized recs | High | High | High | Phase 3 (Q3) |
| Post-show content | Medium | Medium | Medium | Phase 3 (Q3) |
| Venue analytics | Low | High | Medium | Phase 3 (Q3) |
| Multi-city | Very High | Very High | Very High | Phase 4 (Q4+) |

---

## Phase 1: Foundation (Now - Q1)

**Focus**: Core utility that creates daily habits

- [ ] Calendar sync (ICS feed for saved shows)
- [ ] One-click "Add to Google Calendar" button
- [ ] Show reminders (email, 24h before)
- [ ] Basic show save analytics (prep for later features)

**Success metric**: 20% of active users have 3+ saved shows

---

## Phase 2: Social & Artist Engagement (Q2)

**Focus**: Network effects and artist-driven growth

- [ ] "Going" / "Interested" buttons on shows
- [ ] Attendance counts on show cards
- [ ] Artist claim flow (Spotify OAuth verification)
- [ ] Basic artist dashboard (view upcoming shows, basic stats)
- [ ] User follow system (follow artists, maybe users)

**Success metric**:
- 100+ artists claimed
- 30% of shows have 5+ "interested" users

---

## Phase 3: Personalization & Monetization (Q3)

**Focus**: Personalized experience and revenue activation

- [ ] Spotify listening history integration
- [ ] "For You" recommendations section
- [ ] Weekly personalized email digest
- [ ] Venue analytics dashboard (free + paid tiers)
- [ ] Post-show photo uploads
- [ ] Show ratings/reviews

**Success metric**:
- 10% of users connect Spotify
- 5 paying venue subscriptions
- 50% email open rate on personalized digest

---

## Phase 4: Scale (Q4+)

**Focus**: Geographic expansion and platform growth

- [ ] Multi-city data model refactor
- [ ] Tucson launch (test city)
- [ ] City-specific admin roles
- [ ] Regional sponsor infrastructure
- [ ] Mobile app evaluation (PWA vs. native)

**Success metric**:
- 2 cities live
- 1,000+ users in expansion city within 3 months

---

## Technical Debt to Address First

Before building new features, consider addressing:

1. **Analytics foundation**: PostHog event tracking for user behavior (just implemented)
2. **Notification infrastructure**: Push notifications, SMS capability
3. **Image handling**: Scalable upload/storage for user content
4. **Background jobs**: Queue system for emails, notifications, data processing

---

## Competitive Moat

Long-term defensibility comes from:

1. **Local data depth**: Comprehensive show/venue/artist database that takes years to build
2. **User-generated content**: Photos, reviews, attendance history that can't be scraped
3. **Network effects**: "Everyone I know uses it for shows"
4. **Artist relationships**: Verified artists invested in the platform
5. **Historical archive**: "The place to find what happened in AZ music"

The goal is to become **the system of record** for the local music scene.
