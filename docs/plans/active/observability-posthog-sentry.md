# Observability Strategy: PostHog + Sentry

## Overview

This document outlines the recommended observability strategy for Psychic Homily using a combination of **PostHog** (product analytics, session replay, frontend errors) and **Sentry** (backend error tracking, performance monitoring).

## Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| PostHog Frontend | ✅ Implemented | With cookie consent integration |
| Sentry Frontend | ✅ Implemented | Error boundaries, source maps |
| Sentry Backend | ✅ Implemented | SDK init, panic recovery, request context middleware |

## Why Two Tools?

Each tool excels in different areas:

| Capability | PostHog | Sentry | Winner |
|------------|---------|--------|--------|
| Product analytics | Excellent | None | PostHog |
| Session replay | Built-in | Separate product | PostHog |
| Feature flags | Built-in | None | PostHog |
| Frontend JS errors | Good | Excellent | Tie |
| Backend Go errors | Limited | Excellent | Sentry |
| Stack trace depth | Basic | Comprehensive | Sentry |
| Error alerting | Basic | Advanced | Sentry |
| Breadcrumbs | Limited | Full | Sentry |
| Free tier | Very generous | Adequate | PostHog |

**Bottom line:** PostHog tells you *what users are doing*, Sentry tells you *what's breaking*.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Frontend (Next.js)                       │
│                                                                  │
│  ┌─────────────────────────────┐  ┌─────────────────────────────┐│
│  │         PostHog             │  │          Sentry             ││
│  │  - Page views               │  │  - JS exceptions            ││
│  │  - User actions             │  │  - React error boundaries   ││
│  │  - Session recordings       │  │  - Source maps              ││
│  │  - Feature flags            │  │  - Performance (Web Vitals) ││
│  │  - Surveys                  │  │  - Release tracking         ││
│  └─────────────────────────────┘  └─────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Backend (Go)                             │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                        Sentry                                ││
│  │  - Panic recovery                                            ││
│  │  - Error tracking with stack traces                          ││
│  │  - Database query errors                                     ││
│  │  - HTTP handler errors                                       ││
│  │  - Performance monitoring                                    ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

## What Each Tool Does

### PostHog (Frontend Focus)

**Product Analytics**
- Track user journeys through the app
- Funnel analysis (signup → verification → first show submission)
- Retention metrics
- User segmentation

**Session Replay**
- Record user sessions (with privacy controls)
- See exactly what users did before reporting issues
- Identify UX friction points
- Debug frontend issues visually

**Feature Flags**
- Gradual feature rollouts
- A/B testing
- Kill switches for problematic features
- Beta features for specific users

**Surveys**
- In-app feedback collection
- NPS scores
- Feature requests

**Error Tracking (Basic)**
- JavaScript exceptions
- Links errors to session replays
- Good for "what was the user doing when this broke?"

### Sentry (Error Tracking Focus)

**Frontend Error Tracking**
- Detailed stack traces with source maps
- Breadcrumbs (user actions leading to error)
- Error grouping and deduplication
- Release health tracking

**Backend Error Tracking**
- Go panic recovery
- Full stack traces
- Request context (headers, body, user)
- Database error tracking
- Distributed tracing

**Alerting**
- Slack/Discord/email notifications
- Spike detection
- Error rate thresholds
- On-call integrations

**Performance Monitoring**
- Web Vitals (LCP, FID, CLS)
- API latency tracking
- Database query performance
- Transaction tracing

## Free Tier Comparison

### PostHog Free Tier (Monthly)
- 1,000,000 product analytics events
- 5,000 session recordings
- 1,000,000 feature flag requests
- 100,000 error events
- 250 survey responses
- **Unlimited users/seats**

### Sentry Free Tier (Monthly)
- 5,000 errors
- 10,000 performance transactions
- 1 user
- 30-day retention

### Cost at Scale

For a small-to-medium app like Psychic Homily, both free tiers should be sufficient for the first year. If you exceed limits:

**PostHog Paid:**
- $0.00031 per analytics event (after 1M)
- $0.005 per session recording (after 5K)
- $0.00037 per error (after 100K)

**Sentry Paid (Team):**
- $26/month for 50K errors, 100K transactions

## Implementation Guide

### PostHog Account Setup

1. **Create a PostHog account**
   - Go to [https://posthog.com](https://posthog.com)
   - Click "Get started free" (no credit card required)
   - Sign up with email or GitHub

2. **Create a project**
   - After signing up, you'll be prompted to create a project
   - Name it "Psychic Homily" or similar
   - Select "Web" as your platform

3. **Get your API key**
   - After project creation, you'll see your Project API Key
   - It starts with `phc_` (e.g., `phc_abc123...`)
   - Copy this key for your `.env.local` file

4. **Configure environment variables**
   ```bash
   # frontend/.env.local
   NEXT_PUBLIC_POSTHOG_KEY=phc_your_key_here
   NEXT_PUBLIC_POSTHOG_HOST=https://app.posthog.com
   ```

5. **Enable Session Recordings** (optional but recommended)
   - In PostHog dashboard, go to "Session Replay" in the sidebar
   - Enable session recordings for your project
   - The frontend code already has `maskAllInputs: true` for privacy

### PostHog Frontend Implementation (Already Done)

The PostHog integration is implemented with GDPR-compliant cookie consent:

**Core files:**
- `frontend/lib/posthog.ts` - Initialization with opt-out by default
- `frontend/components/layout/PostHogProvider.tsx` - React integration with consent

**Key features:**
- Tracking disabled by default (`opt_out_capturing_by_default: true`)
- Respects cookie consent via `useCookieConsent()` hook
- Pageview tracking on route changes
- User identification on login (email, admin status)
- Session recordings with masked inputs

**How consent works:**
1. PostHog initializes but does NOT track (opted out by default)
2. When user accepts analytics cookies → `optInPostHog()` called
3. When user rejects/revokes → `optOutPostHog()` called, data reset

**Track custom events:**
```typescript
import { posthog } from '@/lib/posthog'

// Only fires if user has consented to analytics
posthog.capture('show_submitted', {
  venue_id: venue.id,
  artist_count: artists.length,
})
```

### Sentry Setup (Frontend)

1. **Install SDK**
```bash
cd frontend
bun add @sentry/nextjs
bunx @sentry/wizard@latest -i nextjs
```

2. **Configure** (wizard creates these files)
```typescript
// sentry.client.config.ts
import * as Sentry from '@sentry/nextjs'

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
  environment: process.env.NODE_ENV,
  tracesSampleRate: 0.1, // 10% of transactions
  replaysSessionSampleRate: 0, // Use PostHog for replays
  replaysOnErrorSampleRate: 0,
})
```

3. **Error boundary**
```typescript
// app/global-error.tsx
'use client'

import * as Sentry from '@sentry/nextjs'
import { useEffect } from 'react'

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    Sentry.captureException(error)
  }, [error])

  return (
    <html>
      <body>
        <h2>Something went wrong!</h2>
        <button onClick={() => reset()}>Try again</button>
      </body>
    </html>
  )
}
```

### Sentry Setup (Backend Go)

1. **Install SDK**
```bash
cd backend
go get github.com/getsentry/sentry-go
```

2. **Initialize**
```go
// cmd/server/main.go
import "github.com/getsentry/sentry-go"

func main() {
    err := sentry.Init(sentry.ClientOptions{
        Dsn:              os.Getenv("SENTRY_DSN"),
        Environment:      os.Getenv("NODE_ENV"),
        TracesSampleRate: 0.1,
        Release:          "psychic-homily-backend@1.0.0",
    })
    if err != nil {
        log.Fatalf("sentry.Init: %s", err)
    }
    defer sentry.Flush(2 * time.Second)

    // ... rest of main
}
```

3. **Add middleware**
```go
// internal/api/middleware/sentry.go
import (
    "github.com/getsentry/sentry-go"
    sentryhttp "github.com/getsentry/sentry-go/http"
)

func SentryMiddleware() func(http.Handler) http.Handler {
    sentryHandler := sentryhttp.New(sentryhttp.Options{
        Repanic: true,
    })
    return sentryHandler.Handle
}
```

4. **Capture errors in handlers**
```go
// In error handling code
if err != nil {
    sentry.CaptureException(err)
    // ... return error response
}

// With user context
if hub := sentry.GetHubFromContext(ctx); hub != nil {
    hub.Scope().SetUser(sentry.User{
        ID:    fmt.Sprintf("%d", user.ID),
        Email: *user.Email,
    })
}
```

## Environment Variables

### Frontend (.env.local)
```bash
# PostHog
NEXT_PUBLIC_POSTHOG_KEY=phc_xxxxxxxxxxxxx
NEXT_PUBLIC_POSTHOG_HOST=https://app.posthog.com

# Sentry
NEXT_PUBLIC_SENTRY_DSN=https://xxxxx@o1234.ingest.sentry.io/xxxxx
SENTRY_AUTH_TOKEN=sntrys_xxxxx  # For source maps upload
```

### Backend (.env)
```bash
# Sentry
SENTRY_DSN=https://xxxxx@o1234.ingest.sentry.io/xxxxx
```

## Verifying PostHog Integration

1. **Cookie consent gating**
   - Open site in incognito mode
   - Open browser DevTools → Network tab
   - Filter by "posthog" or "app.posthog.com"
   - Verify NO requests until you click "Accept" on the cookie banner

2. **Pageview tracking**
   - Accept cookies
   - Navigate between pages
   - In PostHog dashboard → Events → filter by `$pageview`
   - Verify pageviews appear with correct URLs

3. **Consent revocation**
   - Click footer link to open cookie preferences
   - Disable analytics cookies
   - Verify no more PostHog requests in Network tab

4. **User identification**
   - Log in to your account
   - In PostHog dashboard → Persons
   - Find your user with email and `is_admin` property

5. **Session replay**
   - With cookies accepted, browse the site
   - In PostHog dashboard → Session Replay
   - Verify recordings appear (may take a few minutes)

## Workflow: Debugging an Issue

### Scenario: User reports "I can't save shows"

1. **Check PostHog first**
   - Find the user's session replay
   - Watch what they were doing
   - See if there's an error in the console
   - Check if the issue is UX-related (button not visible, etc.)

2. **Check Sentry for technical details**
   - Search for errors from that user
   - Review stack traces
   - Check breadcrumbs for API calls
   - See if it's a backend error

3. **Correlate data**
   - PostHog shows: User clicked "Save" button 3 times
   - Sentry shows: 409 Conflict error from `/saved-shows` endpoint
   - Root cause: Race condition in save handler

## Privacy Considerations

### PostHog
- Session replay masks sensitive inputs by default
- Can exclude specific pages from recording
- User can opt-out via cookie consent

### Sentry
- Scrub sensitive data from payloads
- Don't send PII in error messages
- Configure data scrubbing rules

```typescript
// Sentry data scrubbing
Sentry.init({
  beforeSend(event) {
    // Remove email from user data
    if (event.user?.email) {
      event.user.email = '[REDACTED]'
    }
    return event
  },
})
```

## When to Use Each Tool

| Question | Tool |
|----------|------|
| "How many users signed up this week?" | PostHog |
| "What's our signup → submission conversion rate?" | PostHog |
| "Why did the app crash for this user?" | Sentry |
| "What was the user doing before the crash?" | PostHog (replay) |
| "Is this error affecting many users?" | Sentry |
| "Should we roll out feature X to everyone?" | PostHog (flags) |
| "What's our API latency p95?" | Sentry |
| "Where do users drop off in the submission flow?" | PostHog |

## Alternative: PostHog Only

If you want to simplify to one tool, PostHog can handle basic error tracking. Trade-offs:

**Pros:**
- One dashboard
- Errors linked to session replays
- Simpler setup

**Cons:**
- No backend Go error tracking
- Less detailed stack traces
- Basic alerting only
- No performance monitoring

**Recommendation:** Start with both. The free tiers are generous, and having deep backend error tracking from Sentry is valuable for a production app. You can always consolidate later if it feels like overkill.

## Resources

- [PostHog Docs](https://posthog.com/docs)
- [PostHog Next.js Integration](https://posthog.com/docs/libraries/next-js)
- [Sentry Next.js SDK](https://docs.sentry.io/platforms/javascript/guides/nextjs/)
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)
- [PostHog vs Sentry Comparison](https://posthog.com/blog/best-sentry-alternatives)
