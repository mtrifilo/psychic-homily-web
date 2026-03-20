# Deprecated

This standalone discovery app (Bun + Playwright) is **deprecated** in favor of the API-first data pipeline built into the main application.

## Why

The Playwright-based scrapers in this app were brittle, slow, and required per-venue provider code. The new pipeline uses:

- **Tiered rendering** (static HTTP, chromedp dynamic, screenshot) with automatic detection
- **AI extraction** (Claude Haiku) that works on any venue page without per-venue code
- **iCal/RSS feed parsing** as the cheapest extraction tier
- **Change detection** to skip unchanged pages and reduce costs
- **Automated scheduling** with circuit breakers and anomaly detection

## What to use instead

All pipeline management is now in the **Admin Console > Data Pipeline** tab:

- Per-venue configuration (calendar URL, render method, source type, extraction notes)
- Manual extraction triggers (dry run and live import)
- Cross-venue import history with source type, event counts, and status
- Rejection stats and approval rate tracking
- Run history per venue

Access it at `/admin?tab=pipeline` when logged in as an admin.

## Backend endpoints

The pipeline is powered by these admin API endpoints:

- `GET /admin/pipeline/venues` - List configured venues
- `GET /admin/pipeline/imports` - Cross-venue import history
- `POST /admin/pipeline/extract/{venue_id}` - Trigger extraction
- `GET /admin/pipeline/venues/{venue_id}/stats` - Rejection stats
- `GET /admin/pipeline/venues/{venue_id}/runs` - Per-venue run history
- `PUT /admin/pipeline/venues/{venue_id}/config` - Update venue config
