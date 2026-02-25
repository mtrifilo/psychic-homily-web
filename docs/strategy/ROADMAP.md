# Cross-Track Roadmap

> High-level priorities across all tracks. For track-specific details, see the individual track files.

## Current Priority Order

| Priority | Track | What | Why | Details |
|----------|-------|------|-----|---------|
| 1 | Web | Phase 1 features (ICS feed, calendar, reminders) | Artists page shipped; now building daily-habit utility features | [strategy/web.md](web.md) |
| 2 | iOS | Polish, test, submit to App Store | App is built but needs QA and submission; requires Apple Developer enrollment | [strategy/ios.md](ios.md) |
| 3 | Discovery | Provider reliability audit, expand coverage | Stable but fragile; more venues = more value for users | [strategy/discovery.md](discovery.md) |

## Q1 2026: Launch & Foundation

**Theme**: Ship the web app, get iOS into TestFlight, stabilize discovery.

### Web (Primary Focus)
- [x] **Artists list page (`/artists`)** — show counts, search, multi-city filters, compact grid
- [x] **Venues page search + multi-city filters** — VenueSearch, shift+click multi-select
- [ ] Email preferences UI (deferred until notification emails exist)
- [ ] Calendar sync (ICS feed for saved shows)
- [ ] One-click "Add to Google Calendar"
- [ ] Show reminders (email, 24h before)
- [ ] AI/Agent API Phase 1 (date range filtering, rate limit headers)

### iOS
- [ ] Enroll in Apple Developer Program ($99/year)
- [ ] Re-enable capabilities (Sign in with Apple, App Groups, Keychain Sharing)
- [ ] Polish & error states (Phase 8)
- [ ] TestFlight internal beta

### Discovery
- [ ] Audit all 5 providers against live sites
- [ ] Fix any broken scrapers
- [ ] Identify next venues to add

## Q2 2026: Growth

**Theme**: Social features on web, App Store launch for iOS, automated discovery.

### Web
- [ ] "Going" / "Interested" buttons on shows
- [ ] Artist claim flow (Spotify OAuth verification)
- [ ] User follow system
- [ ] AI/Agent API Phase 2 (full-text search, genre filtering)

### iOS
- [ ] App Store submission (Phase 9)
- [ ] Post-launch bug fixes and polish
- [ ] Push notifications (V2)

### Discovery
- [ ] New venue providers (expand AZ coverage)
- [ ] Automated scraping schedule
- [ ] Error alerting (Discord/Sentry on failures)

## Q3 2026: Personalization & Monetization

**Theme**: Revenue activation, personalized experience.

### Web
- [ ] Spotify listening history → "For You" recommendations
- [ ] Weekly personalized email digest
- [ ] Venue analytics dashboard (free + paid tiers)
- [ ] Post-show photo uploads

### iOS
- [ ] Feature parity with web (social features, recommendations)
- [ ] Offline mode, widgets

### Discovery
- [ ] Multi-city provider support (Tucson venues)
- [ ] Auto-import for high-confidence matches

## Q4 2026+: Scale

**Theme**: Geographic expansion.

- [ ] Multi-city data model refactor
- [ ] Tucson launch (test expansion city)
- [ ] City-specific admin roles
- [ ] AI/Agent API Phase 3 (MCP server, OpenAI plugin)

## Open Decisions

- **iOS distribution**: TestFlight beta vs. direct App Store submission timing
- **Multi-city architecture**: First-class city entity vs. tag-based approach
- **Monetization model**: Venue subscriptions vs. sponsored placements vs. hybrid
- **Mobile strategy**: Continue native iOS or evaluate PWA

## Risks

- **Single-person team**: Bus factor of 1 for all components
- **Venue provider fragility**: Scrapers break when venue sites change
- **Cold start problem**: Social features need critical mass to be valuable
- **App Store approval**: iOS review process timeline is unpredictable
