# Deployment

**This file is a pointer. Its former contents described a VPS + systemd + Hugo/Netlify architecture that is no longer how this project deploys — following them would be actively wrong.**

Production runs on **Railway** (Go backend, built from `backend/Dockerfile`) and **Vercel** (Next.js frontend). Both production targets deploy from the `production` branch; stage deploys from `main`. Database migrations apply automatically on container boot via `docker-entrypoint.sh`.

The current deploy procedure — topology, preflight checks, release steps, environment-variable inventory, rollback, and known integration quirks — lives in:

- `docs/runbooks/production-deploy.md` — the full runbook
- `.claude/skills/psy-deploy-prod/SKILL.md` — the executable release checklist

Both are agent-facing and gitignored; ask the repo owner if they are not present in your checkout.
