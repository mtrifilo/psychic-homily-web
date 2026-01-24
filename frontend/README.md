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

See `/dev-docs/authentication-overhaul.md` for detailed documentation.

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
