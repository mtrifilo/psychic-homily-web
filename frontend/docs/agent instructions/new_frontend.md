# Psychic Homily - Frontend Agent Instructions

## Project Overview

**Psychic Homily** is a concert listing website focused on the Arizona music community. The website features:

- **Upcoming Shows** - Concert listings with artist, venue, date/time, price, and age requirements
- **Blog** - Music-related articles with Bandcamp and SoundCloud embeds
- **DJ Sets** - Curated mixes with SoundCloud embeds
- **Show Submissions** - Authenticated users can submit new show listings
- **User Authentication** - Email/password and OAuth login

**Website:** https://psychichomily.com

---

## Architecture Overview

The project is undergoing a migration from a Hugo static site to a Next.js application:

| Layer        | Legacy                                                | Current (New)                              |
| ------------ | ----------------------------------------------------- | ------------------------------------------ |
| **Frontend** | Hugo (root level) + React components (`/components/`) | Next.js App Router (`/frontend/`)          |
| **Backend**  | N/A                                                   | Go API with Huma framework (`/backend/`)   |
| **Database** | N/A                                                   | PostgreSQL via Docker                      |
| **Content**  | Markdown files (`/content/`)                          | Shared with Hugo (read from `../content/`) |

### Key Technology Stack

#### Frontend (Next.js - `/frontend/`)

- **Framework:** Next.js 16 with App Router
- **Language:** TypeScript
- **Styling:** Tailwind CSS v4
- **State/Data Fetching:** TanStack Query (React Query)
- **Forms:** TanStack Form + Zod validation
- **UI Components:** Radix UI primitives + shadcn/ui patterns
- **Authentication:** HTTP-only cookies (JWT)
- **Package Manager:** bun

#### Backend (Go - `/backend/`)

- **Framework:** Huma v2 (REST API with OpenAPI)
- **Router:** Chi
- **ORM:** GORM
- **Database:** PostgreSQL 17.5
- **Auth:** JWT + OAuth (via Goth)
- **Containerization:** Docker & Docker Compose

---

## Directory Structure

```
psychic-homily-web/
├── frontend/                    # NEW Next.js application
│   ├── app/                     # Next.js App Router pages
│   │   ├── api/[...path]/       # API proxy for development
│   │   ├── auth/                # Login/signup page
│   │   ├── blog/                # Blog listing and posts
│   │   ├── categories/          # Blog category pages
│   │   ├── dj-sets/             # DJ sets/mixes pages
│   │   ├── shows/               # Shows listing page
│   │   ├── submissions/         # Authenticated show submission
│   │   ├── layout.tsx           # Root layout with providers
│   │   ├── nav.tsx              # Navigation component
│   │   └── page.tsx             # Homepage
│   ├── components/              # React components
│   │   ├── blog/                # Blog-specific components
│   │   ├── forms/               # Form components (ArtistInput, VenueInput, ShowForm)
│   │   ├── ui/                  # shadcn/ui base components
│   │   ├── home-show-list.tsx   # Homepage show preview
│   │   ├── show-list.tsx        # Full shows list (with admin edit buttons)
│   │   └── providers.tsx        # QueryClient + Auth providers
│   ├── lib/                     # Utilities and hooks
│   │   ├── api.ts               # API client configuration
│   │   ├── blog.ts              # Blog content utilities
│   │   ├── mixes.ts             # DJ sets content utilities
│   │   ├── context/             # React contexts (AuthContext with is_admin)
│   │   ├── errors/              # Typed error classes (AuthError, ShowError)
│   │   ├── hooks/               # Custom hooks (useShows, useShowUpdate, useAuth, etc.)
│   │   ├── types/               # TypeScript type definitions
│   │   └── utils/               # Utility functions (timeUtils, authLogger, showLogger)
│   └── public/                  # Static assets
│
├── backend/                     # Go API server
│   ├── cmd/
│   │   ├── server/main.go       # Application entry point
│   │   └── seed/main.go         # Database seeding
│   ├── internal/
│   │   ├── api/
│   │   │   ├── handlers/        # HTTP handlers (show, artist, venue, auth)
│   │   │   ├── middleware/      # JWT, RequestID middleware
│   │   │   └── routes/          # Route definitions
│   │   ├── auth/                # OAuth (Goth) setup
│   │   ├── config/              # Environment configuration
│   │   ├── errors/              # Typed error codes (auth.go, show.go)
│   │   ├── logger/              # Structured logging (slog-based)
│   │   ├── models/              # GORM models
│   │   └── services/            # Business logic
│   ├── db/migrations/           # SQL migrations (golang-migrate)
│   ├── docker-compose.yml       # Development Docker setup
│   └── scripts/                 # Deployment and backup scripts
│
├── components/                  # LEGACY React components (Vite)
│   └── src/                     # Being migrated to /frontend/
│
├── content/                     # Markdown content (shared with Hugo)
│   ├── blog/                    # Blog posts
│   ├── mixes/                   # DJ sets/mixes
│   └── shows/                   # Legacy show data
│
├── config/                      # Hugo configuration
├── layouts/                     # Hugo templates
└── themes/                      # Hugo themes
```

---

## Backend API

### Base URL

- **Development:** `http://localhost:8080` (via proxy: `http://localhost:3000/api`)
- **Production:** `https://api.psychichomily.com`

### API Endpoints

#### Authentication (Public)

| Method | Endpoint                    | Description                |
| ------ | --------------------------- | -------------------------- |
| POST   | `/auth/login`               | Email/password login       |
| POST   | `/auth/register`            | Create new account         |
| POST   | `/auth/logout`              | Clear auth cookie          |
| GET    | `/auth/login/{provider}`    | OAuth login (google, etc.) |
| GET    | `/auth/callback/{provider}` | OAuth callback             |

#### Authentication (Protected)

| Method | Endpoint        | Description              |
| ------ | --------------- | ------------------------ |
| GET    | `/auth/profile` | Get current user profile |
| POST   | `/auth/refresh` | Refresh JWT token        |

#### Shows

| Method | Endpoint           | Auth      | Description                    |
| ------ | ------------------ | --------- | ------------------------------ |
| GET    | `/shows`           | Public    | List all shows                 |
| GET    | `/shows/upcoming`  | Public    | Upcoming shows with pagination |
| GET    | `/shows/{show_id}` | Public    | Get single show                |
| POST   | `/shows`           | Protected | Create new show                |
| PUT    | `/shows/{show_id}` | Protected | Update show                    |
| DELETE | `/shows/{show_id}` | Protected | Delete show                    |

#### Artists & Venues

| Method | Endpoint                 | Description            |
| ------ | ------------------------ | ---------------------- |
| GET    | `/artists/search?query=` | Search artists by name |
| GET    | `/venues/search?query=`  | Search venues by name  |

#### System

| Method | Endpoint        | Description           |
| ------ | --------------- | --------------------- |
| GET    | `/health`       | Health check          |
| GET    | `/openapi.json` | OpenAPI specification |

---

## Database Schema

### Core Tables

- **shows** - Concert events (title, event_date, city, state, price, age_requirement, description)
- **artists** - Bands/performers (name, city, state, social links)
- **venues** - Concert venues (name, address, city, state, social links)
- **show_artists** - Junction table with position/headliner info
- **show_venues** - Junction table for multi-venue events

### Auth Tables

- **users** - User accounts (email, password_hash, name, is_admin)
- **oauth_accounts** - OAuth provider connections
- **user_preferences** - User settings

---

## Frontend Architecture

### Data Fetching Pattern

The frontend uses TanStack Query for all API interactions:

```typescript
// lib/hooks/useShows.ts
export const useUpcomingShows = (options = {}) => {
  return useQuery({
    queryKey: queryKeys.shows.list(options),
    queryFn: () => apiRequest<UpcomingShowsResponse>(endpoint),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
```

### Authentication Flow

1. **Login/Register** → Server sets HTTP-only `auth_token` cookie
2. **Profile fetch** → Cookie sent automatically via `credentials: 'include'`
3. **AuthContext** → Provides `user`, `isAuthenticated`, `isLoading` to components
4. **Protected routes** → Check auth state and redirect to `/auth` if needed
5. **Admin detection** → `user.is_admin` from profile enables admin-only features

### Admin Features

Admin users (`user.is_admin === true`) have additional capabilities:

- **Edit shows** - Pencil icon appears on show cards in the show list
- **Inline editing** - Clicking edit expands the ShowForm below the card, pre-filled with current data
- **Future** - Admin dashboard for managing users, venues, artists

### API Proxy (Development)

To handle same-origin cookie requirements, the frontend includes an API proxy at `/app/api/[...path]/route.ts` that forwards requests to `http://localhost:8080` and modifies cookies for same-origin compatibility.

### Content Loading

Blog posts and DJ sets are read from the Hugo content directory at build/runtime:

```typescript
// lib/blog.ts
const BLOG_CONTENT_PATH = path.join(process.cwd(), '..', 'content', 'blog')
```

Hugo shortcodes are converted to MDX components:

- `{{< bandcamp ... >}}` → `<Bandcamp ... />`
- `{{< soundcloud ... >}}` → `<SoundCloud ... />`

---

## TypeScript Types

### Key Type Definitions

```typescript
// lib/types/show.ts
interface ShowResponse {
  id: number
  title: string
  event_date: string // ISO date
  city?: string | null
  state?: string | null
  price?: number | null
  age_requirement?: string | null
  description?: string | null
  venues: VenueResponse[]
  artists: ArtistResponse[]
  created_at: string
  updated_at: string
}

// lib/types/artist.ts
interface Artist {
  id: number
  name: string
  state: string | null
  city: string | null
  social: ArtistSocial // instagram, bandcamp, spotify, etc.
}

// lib/types/venue.ts
interface Venue {
  id: number
  name: string
  address: string | null
  city: string
  state: string
  verified: boolean
}
```

---

## Development Setup

### Prerequisites

- Node.js 20+ or Bun
- Docker & Docker Compose
- Go 1.24+ (for backend development)

### Starting Development

```bash
# 1. Start backend database and API
cd backend
docker compose up -d db migrate  # Start PostgreSQL + run migrations
go run cmd/server/main.go        # Start API on :8080

# 2. Start frontend
cd frontend
bun install
bun dev                          # Start Next.js on :3000
```

### Environment Variables

**Frontend** (`frontend/.env.local`):

```env
NEXT_PUBLIC_API_URL=http://localhost:8080  # Optional, uses proxy by default
```

**Backend** (`backend/.env.development`):

```env
API_ADDR=127.0.0.1:8080
DATABASE_URL=postgres://psychicadmin:secretpassword@localhost:5432/psychicdb
CORS_ALLOWED_ORIGINS=http://localhost:3000
JWT_SECRET=your-secret-key
```

---

## Coding Conventions

### Component Structure

- Use functional components with hooks
- Place page components in `/app/` directory
- Reusable components go in `/components/`
- Use `'use client'` directive for interactive components

### Styling

- Tailwind CSS utility classes
- Dark mode support via `next-themes`
- CSS variables for theming (defined in `globals.css`)

### Forms

- TanStack Form for form state management
- Zod schemas for validation
- Custom form components in `/components/forms/`
- **ShowForm** - Reusable form for creating/editing shows:
  - `mode: 'create' | 'edit'` - Determines submit behavior
  - `initialData?: ShowResponse` - Pre-fills form for editing
  - `onSuccess?: () => void` - Callback after successful submit
  - `onCancel?: () => void` - Callback for cancel (edit mode only)

### API Calls

- Always use hooks from `/lib/hooks/`
- Use `apiRequest()` helper from `/lib/api.ts`
- Handle loading and error states

Key hooks:

- `useUpcomingShows()` - Fetch upcoming shows with pagination
- `useShowSubmit()` - Create new show (POST)
- `useShowUpdate()` - Update existing show (PUT)
- `useProfile()` - Get current user profile (includes `is_admin`)
- `useLogin()`, `useRegister()`, `useLogout()` - Auth mutations

### Error Handling

The codebase uses typed error classes and structured logging for debugging:

#### Error Classes (`/lib/errors/`)

**AuthError** - Authentication-related errors:

```typescript
import { AuthError, AuthErrorCode, isAuthError } from '@/lib/errors'

try {
  await login(email, password)
} catch (error) {
  if (isAuthError(error)) {
    if (error.isExpired) {
      // Token expired, redirect to login
    }
    console.log(error.code) // e.g., "INVALID_CREDENTIALS"
    console.log(error.requestId) // e.g., "a1b2c3d4-..."
  }
}
```

Error codes: `INVALID_CREDENTIALS`, `TOKEN_EXPIRED`, `TOKEN_INVALID`, `TOKEN_MISSING`, `UNAUTHORIZED`, `USER_NOT_FOUND`, `USER_EXISTS`

**ShowError** - Show-related errors:

```typescript
import { ShowError, ShowErrorCode, isShowError } from '@/lib/errors'

try {
  await updateShow(showId, updates)
} catch (error) {
  if (isShowError(error)) {
    if (error.isValidationError) {
      // Show validation errors to user
    }
    if (error.isRetryable) {
      // Service unavailable, can retry
    }
  }
}
```

Error codes: `SHOW_NOT_FOUND`, `SHOW_CREATE_FAILED`, `SHOW_UPDATE_FAILED`, `SHOW_DELETE_FAILED`, `SHOW_INVALID_ID`, `SHOW_VALIDATION_FAILED`, `VENUE_REQUIRED`, `ARTIST_REQUIRED`, `INVALID_EVENT_DATE`

#### Logging Utilities (`/lib/utils/`)

**authLogger** - Authentication event logging:

```typescript
import { authLogger } from '@/lib/utils/authLogger'

authLogger.loginAttempt(email)
authLogger.loginSuccess(userId, requestId)
authLogger.loginFailed(errorCode, message, requestId)
authLogger.debug('Custom message', { data }, requestId)
```

**showLogger** - Show event logging:

```typescript
import { showLogger } from '@/lib/utils/showLogger'

showLogger.submitAttempt({ venueCount, artistCount, city, state })
showLogger.submitSuccess(showId, requestId)
showLogger.submitFailed(errorCode, message, requestId)
showLogger.updateAttempt(showId, updateFields)
showLogger.updateSuccess(showId, requestId)
showLogger.updateFailed(showId, errorCode, message, requestId)
```

Debug logs only appear in development mode (`NODE_ENV === 'development'`).

#### Request ID Correlation

Errors include a `requestId` that matches backend logs for debugging:

- Frontend logs: `[Auth:WARN] [a1b2c3d4] Login failed { errorCode: "INVALID_CREDENTIALS" }`
- Backend logs: `level=WARN msg=login_failed request_id=a1b2c3d4-...`

The request ID flows through:

1. **Frontend**: `apiRequest()` reads `X-Request-ID` header from responses
2. **Backend**: `RequestIDMiddleware` generates/propagates the ID
3. **Error responses**: Include `(request_id: uuid)` in error messages

---

## Migration Status

### Completed ✅

- Homepage with show list, blog preview, DJ set preview
- Full shows listing page (`/shows`)
- Blog listing and single post pages (`/blog`, `/blog/[slug]`)
- DJ sets pages (`/dj-sets`, `/dj-sets/[slug]`)
- Categories taxonomy (`/categories/[category]`)
- User authentication (login, register, logout)
- Show submission form (`/submissions`)
- Admin show editing (inline edit form on show cards)
- Reusable ShowForm component (create/edit modes)
- Navigation with responsive mobile menu
- Dark mode toggle
- API proxy for development

### In Progress / TODO 🚧

- Venue detail pages (`/venues/[id]`)
- Artist detail pages
- Show detail pages (`/shows/[id]`)
- Admin dashboard
- Search functionality
- SEO meta tags (currently using placeholder)
- Production deployment configuration

### Legacy (Hugo) - Still Active

- Hugo continues to serve some content during migration
- Content files in `/content/` are shared between both systems
- Hugo templates in `/layouts/` for reference

---

## Common Tasks

### Adding a New Page

1. Create file in `/frontend/app/your-page/page.tsx`
2. Add to navigation in `/frontend/app/nav.tsx` if needed

### Adding a New API Hook

1. Create hook in `/lib/hooks/useYourHook.ts`
2. Define types in `/lib/types/`
3. Add query key to `/lib/queryClient.ts`

### Adding a New UI Component

1. Use shadcn/ui CLI or create in `/components/ui/`
2. Follow Radix UI patterns for accessibility

### Creating Database Migrations

```bash
cd backend
migrate create -ext sql -dir db/migrations -seq your_migration_name
# Edit the generated .up.sql and .down.sql files
docker compose run --rm migrate
```

---

## Troubleshooting

### API Connection Issues

- Ensure backend is running on port 8080
- Check CORS settings in backend config
- Verify proxy is working: check browser Network tab

### Auth Cookie Issues

- Cookies require same-origin in development (use proxy)
- Check `SameSite` and `Secure` cookie attributes
- Clear cookies and re-login

### Content Not Loading

- Ensure `/content/` directory exists with markdown files
- Check file paths in `lib/blog.ts` and `lib/mixes.ts`
- Verify frontmatter format in markdown files

### Debugging API Errors

1. **Check browser console** for `[Auth:*]` or `[Show:*]` log entries
2. **Note the request ID** in square brackets (e.g., `[a1b2c3d4]`)
3. **Search backend logs** for the same request ID:
   ```bash
   docker compose logs api | grep "a1b2c3d4"
   ```
4. **Error codes** in the message (e.g., `[SHOW_UPDATE_FAILED]`) indicate the failure type
5. **Typed errors** can be caught and handled programmatically:
   ```typescript
   if (isShowError(error) && error.isValidationError) {
     // Handle validation failure
   }
   ```

---

## Useful Commands

```bash
# Frontend
bun dev                    # Start development server
bun build                  # Production build
bun lint                   # Run ESLint

# Backend
go run cmd/server/main.go  # Start API server
docker compose up -d       # Start all services
docker compose logs -f     # View logs

# Database
docker compose exec db psql -U psychicadmin -d psychicdb  # Connect to DB
```
