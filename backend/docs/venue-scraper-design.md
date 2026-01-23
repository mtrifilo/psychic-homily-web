# Venue Scraper System Design

## Overview

Automated discovery of upcoming shows from venue websites to populate pending shows for admin review. The goal is to eliminate manual calendar checking across multiple venues while ensuring quality control through admin approval.

## Proof of Concept: Valley Bar

**Status:** Working (January 2026)

### Technical Approach

Valley Bar uses TicketWeb for event management. Their calendar page (`/calendar/`) loads event data into a JavaScript array (`all_events`) that populates a FullCalendar widget.

**Scraping method:** Playwright headless browser
- Navigate to calendar page
- Wait for `all_events` variable to be defined
- Extract structured data directly from JavaScript context
- Parse ticket URLs from event dialog elements

### Data Extracted

| Field | Source | Notes |
|-------|--------|-------|
| `id` | `event.id` | TicketWeb event ID |
| `title` | `event.title` | Requires HTML entity decoding |
| `date` | `event.start` | ISO format (YYYY-MM-DD) |
| `venue` | `event.venue` | Embedded in HTML div |
| `doorsTime` | `event.doors` | Format: "Doors: 6:30 pm" |
| `showTime` | `event.displayTime` | Format: "Show: 7:00 pm" |
| `imageUrl` | `event.imageUrl` | Embedded in img tag |
| `ticketUrl` | Dialog elements | TicketWeb URL with event ID |

### Sample Output

```json
{
  "id": "6942",
  "title": "ANIMAL SHIN WITH DREAM 99 / LOOMER / BITTERHAZE",
  "date": "2026-01-20",
  "venue": "Valley Bar",
  "imageUrl": "https://i.ticketweb.com/i/00/12/73/49/54_Original.jpg",
  "doorsTime": "6:30 pm",
  "showTime": "7:00 pm",
  "ticketUrl": "https://www.ticketweb.com/event/animal-shin-with-dream-99-valley-bar-tickets/13981674?pl=valleybar"
}
```

### Project Structure

```
scripts/venue-scraper/
├── package.json
├── bun.lock
├── scrape-valley-bar.js          # Single venue POC (kept for reference)
├── scrape-crescent-ballroom.js   # Single venue POC (kept for reference)
├── scrape-ticketweb-venue.js     # Unified TicketWeb scraper
├── run-scraper.sh                # Wrapper script (scrape + import)
└── output/                       # JSON output directory
    └── .gitkeep

backend/
├── cmd/scrape-import/main.go     # CLI importer tool
├── internal/services/scraper.go  # Scraper service (JSON import, deduplication)
└── db/migrations/
    └── 000010_add_scraper_source_fields.*.sql

deploy/scraper/
├── scraper.service               # Systemd service unit
└── scraper.timer                 # Systemd timer (weekly)
```

**Usage:**
```bash
cd scripts/venue-scraper

# Scrape a specific venue
bun run scrape-ticketweb-venue.js valley-bar

# Scrape all configured venues
bun run scrape-ticketweb-venue.js --all

# Scrape and save to JSON file
bun run scrape-ticketweb-venue.js --all --output ./output

# Full pipeline: scrape + import to database
./run-scraper.sh

# Dry run (no database changes)
./run-scraper.sh --dry-run
```

**Adding a new TicketWeb venue:**
1. Add to the `VENUES` config in `scrape-ticketweb-venue.js`:
```javascript
'new-venue': {
  name: 'New Venue Name',
  url: 'https://example.com/calendar/',
},
```

2. Add to `VenueConfig` in `backend/internal/services/scraper.go`:
```go
"new-venue": {
    Name:    "New Venue Name",
    City:    "Phoenix",
    State:   "AZ",
    Address: "123 Main St",
},
```

---

## Architecture (Implemented)

```
┌──────────────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│ Bun/Playwright   │────▶│ JSON File   │────▶│ Go Importer  │────▶│ PostgreSQL  │
│ Scraper          │     │ (output/)   │     │ CLI Tool     │     │ (pending)   │
└──────────────────┘     └─────────────┘     └──────────────┘     └─────────────┘
        │                                                                │
        │ Systemd timer (weekly, Sundays 3am)                            ▼
        └───────────────────────────────────────────────────────▶ Admin Review
```

### Data Flow

1. **Scraper (Bun/Playwright)**: Scrapes TicketWeb venues, outputs JSON
2. **JSON File**: Intermediate storage with timestamp-based filenames
3. **Go Importer**: Reads JSON, deduplicates, creates pending shows
4. **Database**: Shows stored with `source='scraper'`, `status='pending'`
5. **Admin Review**: Admin panel shows scraped events for approval/rejection

### Deduplication Strategy

1. **Primary**: Unique constraint on `(source_venue, source_event_id)` prevents re-importing same event
2. **Rejected check**: Skip shows where venue + date matches a rejected show
3. **Future**: Fuzzy title matching can be added if needed

---

## Venue Research

### TicketWeb Venues (Stateside Presents)

These venues share identical infrastructure - one scraper works for all:

| Venue | URL | Status | Events |
|-------|-----|--------|--------|
| Valley Bar | valleybarphx.com/calendar | Working | ~110 |
| Crescent Ballroom | crescentphx.com/calendar | Working | ~76 |

**Key insight:** Stateside Presents owns both venues and uses the same TicketWeb + FullCalendar setup. Adding new Stateside venues is trivial - just add URL to config.

### Venues Needing Research

| Venue | URL | Platform | Status |
|-------|-----|----------|--------|
| Rebel Lounge | therebellounge.com | ? | To Research |
| Nile Theater | nilecoffeeshop.com | ? | To Research |
| The Van Buren | thevanburenphx.com | ? | To Research |
| Marquee Theatre | luckymanonline.com | AXS? | To Research |
| Arizona Financial Theatre | livenation.com | Ticketmaster | To Research |

**Research questions for each venue:**
1. What ticketing platform do they use?
2. Is calendar data in JavaScript variables or requires DOM parsing?
3. Are there anti-bot measures (Cloudflare, rate limiting)?
4. Is there an API or iCal feed available?

---

## Implementation Phases

### Phase 1: Single Venue (Valley Bar) ✅ Complete
- [x] Integrate scraper into backend as a runnable task
- [x] Add database migration for source tracking fields
- [x] Existing admin UI shows scraped shows in pending queue
- [x] CLI tool for manual imports

### Phase 2: Scheduling ✅ Complete
- [x] Set up systemd timer (weekly, Sundays 3am)
- [x] Wrapper script handles full pipeline
- [x] Deduplication via unique constraint + rejected show check

### Phase 3: Multi-Venue (In Progress)
- [x] TicketWeb venues share one scraper (Valley Bar, Crescent Ballroom)
- [ ] Research additional venue calendars
- [ ] Build scrapers for non-TicketWeb venues
- [ ] Venue configuration (enable/disable per venue)

### Phase 4: Improvements (Future)
- [ ] Artist name normalization (fuzzy matching)
- [ ] Auto-linking to existing artists in database
- [ ] Genre detection from event titles
- [ ] Price extraction from ticket pages
- [ ] Discord notification on new scraped shows

---

## Technical Considerations

### Dependencies
- **Playwright** - Headless browser automation
- **Bun** - Runtime for scraper scripts (faster than Node.js)
- **Go** - CLI importer tool

### Deployment (Implemented)
The chosen approach uses **systemd timer** on the server:

```bash
# Install systemd units
sudo cp deploy/scraper/scraper.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now scraper.timer

# Check timer status
systemctl list-timers | grep scraper

# Manual run
sudo systemctl start scraper.service
journalctl -u scraper.service -f
```

This approach:
- Runs on the same server as the backend
- Uses systemd for scheduling (more reliable than cron)
- Logs to journald for easy debugging
- Supports manual triggers via `systemctl start`

### Local Development

```bash
# 1. Run migration
cd backend
make migrate-up

# 2. Run scraper only (outputs JSON)
cd scripts/venue-scraper
bun run scrape-ticketweb-venue.js --all --output ./output

# 3. Import to database (dry run first)
cd backend
go build -o ./scrape-import ./cmd/scrape-import
./scrape-import -input ../scripts/venue-scraper/output/scraped-events-*.json -dry-run

# 4. Import for real
./scrape-import -input ../scripts/venue-scraper/output/scraped-events-*.json

# Or use the wrapper script
cd scripts/venue-scraper
./run-scraper.sh --dry-run  # Test first
./run-scraper.sh            # Run for real
```

### Rate Limiting & Politeness
- Scrape each venue at most once per week
- Add random delays between requests
- Respect robots.txt where possible
- Identify as a real browser (not a bot user agent)

### Error Handling
- Timeout handling (venue sites can be slow)
- Graceful degradation if one venue fails
- Alerting for repeated failures
- Logging for debugging calendar format changes

---

## Ethical Notes

This scraper is designed to:
- **Promote** venues and artists by helping users discover shows
- **Drive ticket sales** by linking directly to ticket pages
- **Reduce friction** for music fans who want to support local music

We are not:
- Competing with venues
- Reselling tickets
- Scraping for commercial data harvesting
- Overwhelming venue servers with requests
