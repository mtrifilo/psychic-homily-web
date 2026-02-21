# Monetization Strategies for Psychic Homily

## Context

Analysis of potential revenue streams for a local music discovery platform, assuming growth to a few thousand users in the medium term.

**Current state**: Free community platform for Arizona music scene
**Target scale**: 2,000-10,000 active users
**Priority**: Preserve community trust while building sustainable revenue

---

## Low-Friction Options (Preserve Community Trust)

### 1. Venue Partnerships / Featured Listings

**What**: Verified venues pay for enhanced visibility and analytics.

**Pricing**: $20-50/month

**Features**:
- Logo display on venue listing
- Featured placement in venue search/browse
- Analytics dashboard (profile views, show saves, click-through to tickets)
- "Featured Venue" badge

**Why it works**:
- Venues already go through verification flow - this extends to paid tier
- Aligned incentives: venues want visibility, users want to discover venues
- Non-intrusive to free users

**Implementation effort**: Medium (analytics tracking, payment integration, admin UI)

---

### 2. Ticket Affiliate Revenue

**What**: Earn commission on ticket sales driven from show listings.

**Typical rates**: 3-5% of ticket price

**Partners to explore**:
- Eventbrite Affiliate Program
- Dice (strong in indie/electronic scenes)
- See Tickets
- Ticketmaster (harder to get into, lower rates)
- Direct venue partnerships for in-house ticketing

**Why it works**:
- Zero friction for users (they're already clicking ticket links)
- Low development effort (swap URLs for tracked affiliate links)
- Scales with traffic
- Show pages already link to ticket sources

**Implementation effort**: Low (URL rewriting, tracking pixels, affiliate account setup)

---

### 3. Sponsored Email Newsletter

**What**: Weekly digest of upcoming shows with a sponsored slot for promoters/venues.

**Pricing**: $50-100 per sponsored slot

**Format**:
- "This Week in AZ Music" - curated picks for the week
- 1-2 sponsored show highlights (clearly labeled)
- Sent to users who opt in to notifications

**Why it works**:
- Email infrastructure already exists (Resend)
- Email preferences already in user model
- Local promoters have marketing budgets but limited reach
- High engagement for curated content

**Implementation effort**: Low-Medium (email template, sponsor management, scheduling)

---

## Medium-Effort Options

### 4. Artist/Promoter "Pro" Tier

**What**: Subscription for artists and show promoters with professional tools.

**Pricing**: $10-15/month

**Features**:
- Analytics: who saved their shows, view counts, traffic sources
- Bulk show submission (CSV upload, recurring events)
- Priority placement in search results
- Embeddable show widget for their own website
- Early access to new features

**Why it works**:
- Artists already exist in system with Spotify/Bandcamp integrations
- Promoters submit shows regularly - save them time
- Analytics are genuinely valuable for booking decisions

**Implementation effort**: Medium-High (analytics infrastructure, widget system, subscription management)

---

### 5. Local Business Advertising

**What**: Flat-rate display ads from music-adjacent local businesses.

**Pricing**: $100-200/month

**Ad placements**:
- Sidebar on show/venue pages
- Footer banner
- "Sponsored by" section on weekly digest

**Target advertisers**:
- Rehearsal spaces
- Recording studios
- Music gear shops
- Local bars/venues (cross-promotion)
- Music lessons/schools

**Why it works**:
- Hyper-relevant to audience
- Simple flat-rate deals (no programmatic complexity)
- Supports local business ecosystem
- Can be tastefully integrated

**Implementation effort**: Low (static ad slots, manual sales)

---

## What to Avoid

| Approach | Why to Avoid |
|----------|--------------|
| Paywalling core features | Kills community growth at this stage |
| Programmatic ads (Google AdSense) | Destroys UX, terrible CPMs for niche local traffic |
| Premium user subscriptions | Hard to justify for event discovery - users can find shows elsewhere |
| Exclusive content | Not enough content depth to gate |
| NFTs/crypto anything | Wrong audience, reputational risk |

---

## Recommended Starting Point

**Phase 1: Ticket Affiliates + Featured Venues**

Both options are:
- Aligned with user interests (they want to find shows)
- Low-to-medium development effort
- Non-intrusive to free experience
- Scale naturally with platform usage

**Phase 2: Sponsored Newsletter**

Once email list reaches 1,000+ subscribers:
- Low effort to add
- Predictable recurring revenue
- Builds direct relationship with promoters

**Phase 3: Pro Tier**

Once you have proven demand:
- Promoters asking for bulk upload
- Artists requesting analytics
- Build based on actual user requests

---

## Revenue Projections (Conservative)

| Stream | Users Needed | Monthly Revenue |
|--------|--------------|-----------------|
| 5 Featured Venues @ $30/mo | 2,000+ | $150 |
| Ticket affiliates (4% on $5K sales) | 3,000+ | $200 |
| 2 Newsletter sponsors @ $75/mo | 1,000+ email | $150 |
| 10 Pro subscriptions @ $12/mo | 5,000+ | $120 |
| 2 Local ads @ $150/mo | 5,000+ | $300 |

**Potential at 5K users**: ~$500-900/month

Not quit-your-job money, but covers hosting costs and provides runway for growth.

---

## Technical Prerequisites

Before implementing paid features:

1. **Payment processing**: Stripe integration for subscriptions and one-time payments
2. **Analytics infrastructure**: Track views, saves, clicks per show/venue/artist
3. **Admin dashboard**: Manage sponsors, featured listings, ad placements
4. **Email segmentation**: Target newsletter to engaged users only

---

## Next Steps

1. [ ] Research ticket affiliate program requirements (Eventbrite, Dice)
2. [ ] Design "Featured Venue" tier and pricing
3. [ ] Prototype analytics tracking for shows/venues
4. [ ] Reach out to 2-3 local venues to gauge interest in featured listings
5. [ ] Set up Stripe account for future payment processing
