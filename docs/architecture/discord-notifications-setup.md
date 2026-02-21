# Discord Notifications Setup

This document tracks the current state and next steps for enabling Discord notifications on Stage and Production environments.

---

## Current State

### Code Status

| Component | Status | Notes |
|-----------|--------|-------|
| Discord service (`internal/services/discord.go`) | Complete | All notification methods implemented |
| Auth handler integration | Complete | `NotifyNewUser` called on registration |
| Show handler integration | Complete | `NotifyNewShow` and `NotifyShowStatusChange` called |
| Admin handler integration | Complete | `NotifyShowApproved` and `NotifyShowRejected` called |
| Config loading | Complete | Reads `DISCORD_WEBHOOK_URL` and `DISCORD_NOTIFICATIONS_ENABLED` |

### Environment Status

| Environment | Backend Deployed | Webhook Created | Env Vars Configured | Tested |
|-------------|------------------|-----------------|---------------------|--------|
| Stage | Yes (Railway) | Yes (`#alerts-stage`) | Yes | Pending test |
| Production | Yes (Railway) | Yes (`#alerts-production`) | Yes | ✅ Working |

---

## Multi-Environment Strategy

Each environment uses a **separate Discord webhook** pointing to a dedicated channel:

| Environment | Discord Channel | Webhook Name |
|-------------|-----------------|--------------|
| Stage | `#alerts-stage` | Psychic Homily Stage |
| Production | `#alerts-production` | Psychic Homily Production |

This allows you to immediately identify which environment generated a notification.

---

## Next Steps

### Stage Environment

#### 1. Create Discord Webhook

- [ ] Create `#alerts-stage` channel in Discord (or use existing channel)
- [ ] Go to **Edit Channel** → **Integrations** → **Webhooks** → **New Webhook**
- [ ] Name it "Psychic Homily Stage"
- [ ] Copy the webhook URL

#### 2. Configure Railway

- [ ] Open Railway dashboard: https://railway.app
- [ ] Select **psychic-homily** project → **Stage** environment
- [ ] Click on the backend service
- [ ] Go to **Variables** tab
- [ ] Add:
  ```
  DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/<your-stage-webhook>
  DISCORD_NOTIFICATIONS_ENABLED=true
  ```
- [ ] Deploy (or wait for auto-redeploy)

#### 3. Test Stage Notifications

- [ ] **New user signup**: Register a new account on stage frontend
  - Expected: Green "New User Registration" embed in `#alerts-stage`
- [ ] **New show submission**: Submit a show as a logged-in user
  - Expected: Blue "New Show" embed with show details
- [ ] **Show approval**: Approve a pending show as admin
  - Expected: Green "Show Approved" embed
- [ ] **Show rejection**: Reject a show as admin with a reason
  - Expected: Red "Show Rejected" embed with reason
- [ ] **Status change**: Change a show's status (e.g., unpublish)
  - Expected: Orange "Show Status Changed" embed

---

### Production Environment

#### 1. Create Discord Webhook

- [ ] Create `#alerts-production` channel in Discord
- [ ] Create webhook named "Psychic Homily Production"
- [ ] Copy the webhook URL

#### 2. Configure Railway

- [ ] Open Railway dashboard: https://railway.app
- [ ] Select **psychic-homily** project → **Production** environment
- [ ] Click on the backend service
- [ ] Go to **Variables** tab
- [ ] Add:
  ```
  DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/<your-production-webhook>
  DISCORD_NOTIFICATIONS_ENABLED=true
  ```
- [ ] Deploy (or wait for auto-redeploy)

#### 4. Test Production Notifications

- [ ] Verify notifications appear in `#alerts-production`
- [ ] Confirm they are distinct from stage notifications

---

## Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DISCORD_WEBHOOK_URL` | Yes | (empty) | Full Discord webhook URL |
| `DISCORD_NOTIFICATIONS_ENABLED` | Yes | `false` | Set to `true` to enable |
| `FRONTEND_URL` | Yes | - | Used for action links in notifications (shared with email config) |

---

## Notification Types

| Event | Color | Triggered By |
|-------|-------|--------------|
| New user registration | Green | `POST /auth/register` or Google OAuth signup |
| New show submission | Blue | `POST /shows` |
| Show status change | Orange | `PATCH /shows/:id` (status field) |
| Show approved | Green | `POST /admin/shows/:id/approve` |
| Show rejected | Red | `POST /admin/shows/:id/reject` |
| Show report | Orange | `POST /shows/:id/report` (user reports issue) |
| New unverified venue | Purple | `POST /shows` (when show creates new venue) |
| Pending venue edit | Purple | `PATCH /venues/:id` (non-admin edit) |

### Show Report Notification

When a user reports a show issue (cancelled, sold out, or inaccurate info), a notification is sent with:
- **Title**: "Show Report: {show title}"
- **Color**: Orange (0xFFA500)
- **Fields**:
  - Report Type (Cancelled / Sold Out / Inaccurate Info)
  - Show title
  - Event date
  - Reporter (hashed email for privacy)
  - Details (if provided, truncated to 200 chars)
  - Action link to admin reports panel

---

## Troubleshooting

### No notifications appearing

1. Check Railway environment variables are set correctly in the Variables tab
2. Verify Railway redeployed after adding variables (check Deployments tab)
3. Test webhook directly:
   ```bash
   curl -X POST "YOUR_WEBHOOK_URL" \
     -H "Content-Type: application/json" \
     -d '{"content": "Test message"}'
   ```
4. Check service logs in Railway: `railway logs --environment stage` or `railway logs --environment production`

### Wrong channel receiving notifications

- Verify each environment has its own unique webhook URL
- Double-check you copied the correct webhook URL for each environment

---

## Related Documentation

- `backend/docs/discord-notifications.md` - Full technical documentation
- `backend/docs/discord-community-setup.md` - Discord server setup guide
- `docs/plans/completed/railway-migration.md` - Railway deployment status and configuration

---

*Created: January 24, 2026*
*Updated: February 3, 2026 - Updated for Railway (migrated from Coolify)*
