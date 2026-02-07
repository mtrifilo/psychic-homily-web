# Venue Discovery

A local web application for discovering venue events and importing them to Psychic Homily.

## Features

- Multi-step UI for discovering events from configured venues
- Quick preview mode to scan events without details
- Full discovery with artist extraction from detail pages
- Dry run mode to preview imports
- Direct import to Stage or Production via API

## Getting Started

### Prerequisites

- [Bun](https://bun.sh/) installed
- Playwright browsers installed: `bunx playwright install chromium`

### Installation

```bash
cd discovery
bun install
```

### Running the App

```bash
bun run dev
```

This starts:
- **Discovery server** on http://localhost:3001 (Playwright-powered discovery)
- **Web UI** on http://localhost:5173 (Vite + React)

Open http://localhost:5173 in your browser.

### Configuration

1. Go to **Settings** in the app
2. Paste your API token (generated from Admin Settings in the web app)
3. Select target environment (Stage or Production)
4. Save settings

## Workflow

1. **Select Venues** - Choose which venues to discover
2. **Preview Events** - Quick scan to see upcoming events
3. **Select Events** - Choose which events to discover in detail
4. **Import** - Preview and import to the backend

## Adding New Venues

Edit `src/lib/config.ts` to add new venues:

```typescript
export const VENUES: VenueConfig[] = [
  {
    slug: 'new-venue',
    name: 'New Venue Name',
    providerType: 'ticketweb', // or implement a new discovery module
    url: 'https://venue-website.com/calendar/',
  },
]
```

If the venue uses a different system than TicketWeb, you'll need to implement a new discovery module in `src/server/providers/`.

## Architecture

```
discovery/
├── src/
│   ├── components/     # React UI components
│   ├── lib/            # Client-side API and config
│   └── server/         # Bun server with Playwright discovery
│       └── providers/  # Venue-specific discovery logic
```

## API Endpoints (Local Server)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/discovery/venues` | GET | List configured venues |
| `/discovery/preview/:slug` | GET | Quick preview of events |
| `/discovery/discover/:slug` | POST | Full discovery with details |
| `/discovery/health` | GET | Health check |
