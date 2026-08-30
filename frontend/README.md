# Psychic Homily Frontend

The frontend for [psychichomily.com](https://psychichomily.com) - a platform for Arizona music discovery, featuring artist profiles, show listings, DJ sets, and a blog.

## Tech Stack

- **Framework:** Next.js 16.1 (App Router, Turbopack)
- **Runtime:** React 19.2
- **Styling:** Tailwind CSS 4, Geist font
- **UI Components:** Radix UI, shadcn/ui patterns
- **Forms:** TanStack Form
- **Data Fetching:** TanStack Query
- **Testing:** Vitest, React Testing Library
- **Package Manager:** Bun

## Prerequisites

- [Bun](https://bun.sh/) (v1.0+)
- The backend server running (see `/backend`)

## Getting Started

1. Install dependencies:
   ```bash
   bun install
   ```

2. Set up environment variables:
   ```bash
   cp .env.example .env.local
   # Edit .env.local with your values
   ```

   Required variables:
   - `ANTHROPIC_API_KEY` - API key for Claude AI features

   Backend-origin variables (all optional locally; the defaults assume a
   backend on `http://localhost:8080`):

   | Variable | What reads it |
   |----------|---------------|
   | `BACKEND_URL` | The `/api` catch-all proxy (`app/api/[...path]/route.ts`): where it forwards to |
   | `NEXT_PUBLIC_API_URL` | The **data** base (`lib/api-base.ts`). Unset in the browser during development means the same-origin `/api` proxy, which is what lets the SameSite=Lax `auth_token` cookie ride along |
   | `NEXT_PUBLIC_OAUTH_BACKEND_URL` | The **OAuth** base (`lib/api-base.ts`). The Google button is a full-page redirect to the backend's `/auth/login/google`, and that redirect does not survive the proxy, so it needs the backend's own origin. Defaults to `NEXT_PUBLIC_API_URL`, then to `http://localhost:8080` |

   Affiliate variables (optional; unset means every outbound ticket link is
   emitted exactly as stored):

   | Variable | What reads it |
   |----------|---------------|
   | `NEXT_PUBLIC_IMPACT_PARTNER_ID` | `lib/tickets/ticketVendors.ts`. Our impact.com partner ID. Setting it tags outbound links to the configured vendors with `?irmp=<id>` on the vendor's own domain and qualifies them with `rel="sponsored"`. Not a secret: the value rides in public URLs |

   `NEXT_PUBLIC_*` values are inlined at **build** time, so turning affiliate
   links on is a redeploy, not an environment edit on a running deployment.
   Setting the variable without rebuilding leaves the shipped bundle carrying
   the old (empty) value, and the only symptom is that links keep rendering
   untagged.

   Set `NEXT_PUBLIC_OAUTH_BACKEND_URL` only when the data base is *not* the
   backend origin. That is most commonly a backend on a non-default port, where
   `NEXT_PUBLIC_API_URL` points at the proxy. Running the E2E suite that way
   needs no extra configuration; `playwright.config.ts` derives it from
   `BACKEND_URL`:

   ```bash
   BACKEND_URL=http://localhost:8099 bun run test:e2e
   ```

3. Run the development server:
   ```bash
   bun dev
   ```

4. Open [http://localhost:3000](http://localhost:3000)

## Scripts

| Command | Description |
|---------|-------------|
| `bun dev` | Start development server (Turbopack) |
| `bun build` | Create production build |
| `bun start` | Start production server |
| `bun lint` | Run ESLint |
| `bun test` | Run tests in watch mode |
| `bun test:run` | Run tests once |
| `bun test:coverage` | Run tests with coverage report |
| `bun test:ui` | Launch Vitest UI |

## Project Structure

```
app/
├── admin/          # Admin dashboard
├── api/            # API routes (proxy to backend)
├── artists/        # Artist profile pages
├── auth/           # Authentication (login, signup)
│   └── magic-link/ # Magic link verification
├── blog/           # Blog posts (MDX)
├── categories/     # Category listing pages
├── collection/     # User collection & settings
├── dj-sets/        # DJ set pages
├── shows/          # Show listings
├── submissions/    # Artist submissions
├── venues/         # Venue pages
└── verify-email/   # Email verification
components/
├── auth/           # Auth components (passkey login/register)
├── settings/       # Settings components (passkeys, password change)
├── ui/             # Shared UI components (shadcn/ui)
└── ...             # Feature-specific components
content/            # MDX blog content
lib/
├── hooks/          # React Query hooks (useAuth, useShows, etc.)
├── context/        # React context providers
└── ...             # Utilities and helpers
docs/               # Project documentation
test/               # Test setup and utilities
```

## Authentication

The app supports multiple authentication methods:

| Method | Description |
|--------|-------------|
| **Email/Password** | Traditional login with password strength validation |
| **Passkeys (WebAuthn)** | Passwordless biometric authentication (Touch ID, Face ID, etc.) |
| **Magic Links** | Email-based passwordless login (requires verified email) |

Key auth features:
- Password strength meter with real-time feedback
- Passkey management in Settings (add, remove, view)
- Password change for authenticated users
- Email verification required for show submissions

See `/docs/plans/completed/authentication-overhaul.md` for detailed documentation.

## Testing

Tests are colocated with source files using `.test.ts` or `.test.tsx` extensions.

```bash
bun test           # Watch mode
bun test:run       # Single run
bun test:coverage  # With coverage
bun test:ui        # Interactive UI
```

## Deployment

Deployed on [Vercel](https://vercel.com). See `docs/vercel-deployment-steps.md` for details.
