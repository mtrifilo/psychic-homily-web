# Psychic Homily

### [https://psychichomily.com](https://psychichomily.com)

A music discovery platform for Arizona and beyond — upcoming shows, venues, and artists with community-driven listings and an admin-curated feed.

<img width="640" alt="Screenshot 2025-02-16 at 3 11 09 AM" src="https://github.com/user-attachments/assets/072b7211-05a4-45e4-9243-0187d37b2aef" />

## Tech Stack

- **Frontend** — Next.js 16, React 19, TanStack Query, Tailwind CSS 4, Shadcn UI
- **Backend** — Go (Chi router, Huma framework, GORM, PostgreSQL)
- **Discovery** — Playwright-based show scraper with provider plugins
- **iOS** — Native Swift app (in progress)
- **Testing** — Vitest (unit), Playwright (E2E), Go `testing` (backend)

## Project Structure

| Directory | Description |
|-----------|-------------|
| `frontend/` | Next.js web application |
| `backend/` | Go API server |
| `discovery/` | Playwright show discovery service |
| `ios/` | Native iOS app |
| `cli/` | Admin CLI tool |
| `deploy/` | Deployment configuration |
| `scripts/` | Utility scripts (test runner, venue discovery) |
| `dev-docs/` | Internal development documentation |

## Prerequisites

- [Go](https://go.dev/) 1.24+
- [Bun](https://bun.sh/) 1.2+
- [Docker](https://www.docker.com/) (for PostgreSQL)
- PostgreSQL 18

## Getting Started

### Backend

```bash
cd backend
docker compose up -d        # Start PostgreSQL
go run ./cmd/server          # Start API server (localhost:8080)
```

### Frontend

```bash
cd frontend
bun install
bun run dev                  # Start dev server (localhost:3000)
```

## Testing

### Backend

```bash
cd backend
go test ./...
```

### Frontend (unit)

```bash
cd frontend
bun run test                 # Watch mode
bun run test:run             # Single run
bun run test:coverage        # With coverage
```

### E2E

Stop the dev backend first (port 8080 must be free), then:

```bash
cd frontend
bun run test:e2e             # Headless
bun run test:e2e:ui          # Interactive Playwright UI
```

### All suites

```bash
./scripts/test-all.sh
```

## License

MIT License

Copyright (c) 2025-2026 Psychic Homily

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
