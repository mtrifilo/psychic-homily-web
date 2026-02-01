# Claude Code Instructions

## Package Managers

- **Frontend**: Always use `bun` (not npm/yarn/pnpm)
  - `bun install`, `bun run dev`, `bun run build`, `bun run test`
- **Backend**: Use `go` commands
  - `go run ./cmd/server`, `go test ./...`

## Project Structure

- `/frontend` - Next.js 16 app (React 19, TanStack Query, Tailwind CSS 4, Vitest)
- `/backend` - Go API (Chi router, Huma, GORM, PostgreSQL)
- `/dev-docs` - Implementation docs and checklists (read these for context on recent work)

## Key Conventions

### Backend
- Database migrations in `/backend/db/migrations/` (numbered `000XXX_name.up.sql` / `.down.sql`)
- API handlers in `/backend/internal/api/handlers/`
- Services in `/backend/internal/services/`
- Models in `/backend/internal/models/`

### Frontend
- App router pages in `/frontend/app/`
- API client and hooks in `/frontend/lib/`
- Tests colocated or in `/frontend/test/`

#### Component Organization
```
/frontend/components/
├── /artists/       # Artist domain (ArtistDetail, ArtistShowsList)
├── /shows/         # Show domain (ShowDetail, ShowList, HomeShowList, dialogs)
├── /venues/        # Venue domain (VenueDetail, VenueList, VenueCard, dialogs)
├── /layout/        # App shell (Footer, Providers, ThemeProvider, ModeToggle)
├── /settings/      # User settings (SettingsPanel, ChangePassword, etc.)
├── /filters/       # Reusable filter components (FilterChip, CityFilters)
├── /forms/         # Form components (ShowForm, VenueEditForm, etc.)
├── /shared/        # Cross-cutting utilities (LoadingSpinner, SaveButton, SocialLinks, MusicEmbed)
├── /admin/         # Admin-only components
├── /auth/          # Authentication components
├── /blog/          # Blog-related components
├── /seo/           # SEO components (JsonLd)
└── /ui/            # Shadcn primitives (Button, Dialog, etc.)
```

- **Domain directories** (artists, shows, venues): Domain-specific components grouped together
- **layout/**: App-level components used in root layout
- **settings/**: User account and settings components
- **filters/**: Generic filter UI components with shared interfaces (e.g., `CityWithCount`)
- **shared/**: Common utilities used across multiple features
- **ui/**: Low-level Shadcn components (don't modify directly)
- Each domain directory has an `index.ts` barrel file for clean imports (e.g., `import { ShowList } from '@/components/shows'`)

### URLs
- Artists, venues, and shows use SEO-friendly slugs (e.g., `/artists/the-national`)
- Handlers support both numeric IDs and slugs for backwards compatibility

## Running Locally

```bash
# Frontend (from /frontend)
bun install
bun run dev

# Backend (from /backend)
go run ./cmd/server
```
