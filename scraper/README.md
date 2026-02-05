# Venue Scraper

A local web application for scraping venue events and importing them to Psychic Homily.

## Features

- Multi-step UI for scraping events from configured venues
- Quick preview mode to scan events without details
- Full scraping with artist extraction from detail pages
- Dry run mode to preview imports
- Direct import to Stage or Production via API

## Getting Started

### Prerequisites

- [Bun](https://bun.sh/) installed
- Playwright browsers installed: `bunx playwright install chromium`

### Installation

```bash
cd scraper
bun install
```

### Running the App

```bash
bun run dev
```

This starts:
- **Scraper server** on http://localhost:3001 (Playwright-powered scraping)
- **Web UI** on http://localhost:5173 (Vite + React)

Open http://localhost:5173 in your browser.

### Configuration

1. Go to **Settings** in the app
2. Paste your API token (generated from Admin Settings in the web app)
3. Select target environment (Stage or Production)
4. Save settings

## Workflow

1. **Select Venues** - Choose which venues to scrape
2. **Preview Events** - Quick scan to see upcoming events
3. **Select Events** - Choose which events to scrape in detail
4. **Import** - Preview and import to the backend

## Adding New Venues

Edit `src/lib/config.ts` to add new venues:

```typescript
export const VENUES: VenueConfig[] = [
  {
    slug: 'new-venue',
    name: 'New Venue Name',
    scraperType: 'ticketweb', // or implement a new scraper
    url: 'https://venue-website.com/calendar/',
  },
]
```

If the venue uses a different system than TicketWeb, you'll need to implement a new scraper in `src/server/scrapers/`.

## Architecture

```
scraper/
├── src/
│   ├── components/     # React UI components
│   ├── lib/            # Client-side API and config
│   └── server/         # Bun server with Playwright scrapers
│       └── scrapers/   # Venue-specific scraping logic
```

## API Endpoints (Local Server)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/scraper/venues` | GET | List configured venues |
| `/scraper/preview/:slug` | GET | Quick preview of events |
| `/scraper/scrape/:slug` | POST | Full scrape with details |
| `/scraper/health` | GET | Health check |
