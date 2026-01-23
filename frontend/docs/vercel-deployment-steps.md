# Vercel Deployment Setup - Remaining Steps

## Branching Strategy

A single Vercel project handles both environments:

| Environment | Branch | Domain | Trigger |
|-------------|--------|--------|---------|
| **Stage** | `main` | `stage.psychichomily.com` | Merges to `main` |
| **Production** | `production` | `psychichomily.com` | Merges to `production` |

**Workflow:**
1. Develop on feature branches
2. Merge to `main` → auto-deploys to Stage
3. When ready, merge `main` to `production` → auto-deploys to Production

---

## Completed by Claude

1. **CORS settings verified** - Backend already supports staging at `backend/internal/config/config.go:237-274`:
   - Production: `psychichomily.com`, `www.psychichomily.com`
   - Stage: `stage.psychichomily.com`, `www.stage.psychichomily.com`
   - Can also set `CORS_ALLOWED_ORIGINS` env var for custom origins

2. **Vercel CLI installed** - `vercel` v50.4.9 is available globally via bun

3. **Build errors fixed** - Added Suspense boundaries to:
   - `frontend/app/collection/page.tsx`
   - `frontend/app/verify-email/page.tsx`

   The commit (`b70955e`) is on the `stage` branch but **not pushed yet**.

---

## Remaining Manual Steps

### 1. Create Production Branch & Push Changes

```bash
# Ensure main has the Suspense fix
git checkout main
git cherry-pick b70955e  # if not already on main
git push origin main

# Create production branch from main
git checkout -b production
git push -u origin production
```

### 2. Vercel Project Setup (Web UI)

1. Go to [vercel.com](https://vercel.com) and sign in
2. Click **Add New Project**
3. Import `psychic-homily-web` from GitHub
4. Configure:
   - **Root Directory:** `frontend`
   - **Framework:** Next.js (auto-detected)
   - **Build Command:** `bun run build` (or leave default `npm run build`)
5. After project creation, go to **Project Settings → Git**:
   - Change **Production Branch** from `main` to `production`

### 3. Environment Variables (Vercel Dashboard)

Set these in **Project Settings → Environment Variables**:

#### Production Environment

| Variable | Value |
|----------|-------|
| `NEXT_PUBLIC_API_URL` | `https://api.psychichomily.com` |
| `NEXT_PUBLIC_URL` | `https://psychichomily.com` |
| `BACKEND_URL` | `https://api.psychichomily.com` |
| `ANTHROPIC_API_KEY` | Your API key |
| `INTERNAL_API_SECRET` | Generate a secure secret |

#### Preview Environment (for staging via `main` branch)

| Variable | Value |
|----------|-------|
| `NEXT_PUBLIC_API_URL` | `https://api-stage.psychichomily.com` (or same as prod) |
| `NEXT_PUBLIC_URL` | `https://stage.psychichomily.com` |
| `BACKEND_URL` | `https://api-stage.psychichomily.com` (or same as prod) |
| `ANTHROPIC_API_KEY` | Your API key |
| `INTERNAL_API_SECRET` | Same or different secret |

### 4. Custom Domains (Vercel Dashboard)

Go to **Project Settings → Domains**:

1. Add `psychichomily.com` → assign to Production (`production` branch)
2. Add `www.psychichomily.com` → redirect to apex domain
3. Add `stage.psychichomily.com` → assign to `main` branch

### 5. DNS Records (name.com)

Log into name.com and update DNS:

| Type | Host | Value | TTL |
|------|------|-------|-----|
| A | @ | `76.76.21.21` | 300 |
| CNAME | www | `cname.vercel-dns.com` | 300 |
| CNAME | stage | `cname.vercel-dns.com` | 300 |

**Note:** Vercel will show exact records needed in their domain configuration UI.

### 6. Backend CORS (Coolify)

Update `CORS_ALLOWED_ORIGINS` environment variable in your backend deployment:

```
https://psychichomily.com,https://www.psychichomily.com,https://stage.psychichomily.com
```

### 7. Disable Netlify

After verifying Vercel deployment works:
1. Test both production and staging environments
2. Disable or delete the Netlify site

---

## Verification Checklist

- [ ] `production` branch created from `main`
- [ ] Both `main` and `production` branches pushed to origin
- [ ] Vercel project created with `frontend` as root directory
- [ ] Production branch set to `production` in Vercel Git settings
- [ ] Environment variables set for Production and Preview
- [ ] Custom domains added in Vercel
- [ ] DNS records updated at name.com
- [ ] SSL certificates issued (automatic via Vercel)
- [ ] Production site loads at psychichomily.com
- [ ] Stage site loads at stage.psychichomily.com
- [ ] API calls work (check browser Network tab)
- [ ] Auth/OAuth flow works
- [ ] Backend CORS updated in Coolify

---

## Optional: Vercel CLI Commands

```bash
# Link project (run from frontend directory)
cd frontend
vercel link

# Deploy current branch to preview
vercel

# Deploy to production (use sparingly - prefer git merge to production branch)
vercel --prod

# Set environment variable interactively
vercel env add ANTHROPIC_API_KEY
```

## Git Workflow Reference

```bash
# Deploy to Stage: merge feature branch to main
git checkout main
git merge feature/my-feature
git push origin main  # Triggers Stage deployment

# Deploy to Production: merge main to production
git checkout production
git merge main
git push origin production  # Triggers Production deployment
```
