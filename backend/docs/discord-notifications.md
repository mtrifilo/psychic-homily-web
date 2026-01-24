# Discord Notifications for Admin Monitoring

This document describes the Discord webhook notification system used to monitor usage events in Psychic Homily.

## Overview

The Discord notification system sends real-time alerts to a Discord channel when key events occur:

- **New user signups** - When someone creates an account
- **New show submissions** - When a user submits a new show
- **Show status changes** - When a show is unpublished, made private, or published
- **Show approvals** - When an admin approves a pending show
- **Show rejections** - When an admin rejects a show (includes rejection reason)

## Architecture

### Design Principles

- **Discord Webhooks** (not bot): Simple HTTP POST requests, no library dependencies
- **Asynchronous**: Fire-and-forget goroutines so API responses aren't delayed
- **Optional**: Graceful no-op if not configured (follows the EmailService pattern)
- **Separate webhooks per environment**: Stage and Production use different channels
- **Log-only errors**: No retries; notifications are informational only

### Flow Diagram

```
User Action → Handler → Discord Service → Discord Webhook → Discord Channel
                ↓
           (async goroutine, non-blocking)
```

### Embed Colors

| Event Type | Color | Hex Code |
|------------|-------|----------|
| New user signup | Green | `0x00FF00` |
| Show approved | Green | `0x00FF00` |
| New show submission | Blue | `0x0066FF` |
| Status change | Orange | `0xFFA500` |
| Show rejected | Red | `0xFF0000` |

## Configuration

### Multi-Environment Strategy

To distinguish between Stage and Production notifications, use **separate Discord channels and webhooks** for each environment:

| Environment | Discord Channel | Webhook Name |
|-------------|-----------------|--------------|
| Stage | `#stage-alerts` | Psychic Homily Stage |
| Production | `#production-alerts` | Psychic Homily Production |

This way, you always know which environment generated a notification at a glance.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `DISCORD_WEBHOOK_URL` | Discord webhook URL for this environment |
| `DISCORD_NOTIFICATIONS_ENABLED` | Set to `true` to enable notifications (default: `false`) |

### Creating Discord Webhooks

Create a separate webhook for each environment:

1. Open your Discord server
2. Create channels for alerts (e.g., `#stage-alerts`, `#production-alerts`)
3. For each channel:
   - Go to **Edit Channel** → **Integrations** → **Webhooks**
   - Click **New Webhook**
   - **Name**: e.g., "Psychic Homily Stage" or "Psychic Homily Production"
   - **Avatar**: Optionally upload a custom icon
   - Click **Copy Webhook URL**

### Coolify Configuration

Environment variables are configured in Coolify for each backend application:

1. Open Coolify dashboard
2. Go to **Projects** → Select your project (e.g., `psychic-homily-stage`)
3. Click on the backend application
4. Go to **Environment Variables**
5. Add:
   ```
   DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN
   DISCORD_NOTIFICATIONS_ENABLED=true
   ```
6. Click **Save** — Coolify will automatically redeploy with the new configuration

Repeat for each environment (Stage, Production) with their respective webhook URLs.

### Behavior When Not Configured

If `DISCORD_NOTIFICATIONS_ENABLED` is `false` or `DISCORD_WEBHOOK_URL` is empty:
- All notification calls become no-ops
- No errors are thrown
- No logs are generated for skipped notifications
- API responses are unaffected

## Notification Content

Each notification includes clickable action links that take admins directly to the relevant page in the web application.

### New User Registration

```
Title: "New User Registration"
Color: Green
Fields:
  - User ID: 123
  - Email: jo***@example.com (masked for privacy)
  - Name: John Doe
```

### New Show Submission

```
Title: "New Show: Band Name at Venue"
Description: Event Date: Jan 15, 2025 8:00 PM
Color: Blue
Fields:
  - Show ID: 456
  - Status: pending
  - Submitter: jo***@example.com
  - Venue(s): The Rebel Lounge
  - Artist(s): Band Name (headliner), Opening Act
  - Actions: [Review Pending Shows](https://psychichomily.com/admin)  ← Clickable link!
```

### Show Status Change

```
Title: "Show Status Changed: Band Name at Venue"
Description: approved → pending
Color: Orange
Fields:
  - Show ID: 456
  - Changed By: us***@example.com
  - Actions: [Review Pending Shows](https://psychichomily.com/admin)  ← When status becomes "pending"
```

### Show Approved

```
Title: "Show Approved: Band Name at Venue"
Color: Green
Fields:
  - Show ID: 456
  - Event Date: Jan 15, 2025
  - Venue(s): The Rebel Lounge
  - Actions: [View on Calendar](https://psychichomily.com)  ← Clickable link!
```

### Show Rejected

```
Title: "Show Rejected: Band Name at Venue"
Description: Reason: Duplicate entry - this show already exists
Color: Red
Fields:
  - Show ID: 456
  - Event Date: Jan 15, 2025
  - Venue(s): The Rebel Lounge
  - Actions: [View Admin Panel](https://psychichomily.com/admin)  ← Clickable link!
```

## Action Links

The notification system includes clickable links that allow admins to quickly navigate to the relevant page:

| Notification Type | Link | Destination |
|-------------------|------|-------------|
| New Show (pending) | "Review Pending Shows" | `/admin` - Admin panel with pending shows |
| Status → Pending | "Review Pending Shows" | `/admin` - Admin panel with pending shows |
| Show Approved | "View on Calendar" | `/` - Main calendar page |
| Show Rejected | "View Admin Panel" | `/admin` - Admin panel |

These links use the `FRONTEND_URL` environment variable (shared with email configuration) to generate the correct URLs for your environment (localhost, staging, or production).

## Admin API Endpoints

The following admin endpoints are available for managing shows:

### Pending Shows
```
GET /admin/shows/pending?limit=50&offset=0
```
Returns shows awaiting approval.

### Rejected Shows
```
GET /admin/shows/rejected?limit=50&offset=0&search=keyword
```
Returns previously rejected shows. Supports searching by show title or rejection reason.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Number of shows to return (max 100) |
| `offset` | int | 0 | Pagination offset |
| `search` | string | - | Search by title or rejection reason (case-insensitive) |

**Example Response:**
```json
{
  "shows": [
    {
      "id": 123,
      "title": "Band Name at Venue",
      "event_date": "2025-02-15T20:00:00Z",
      "status": "rejected",
      "rejection_reason": "Duplicate entry - this show already exists",
      "venues": [...],
      "artists": [...]
    }
  ],
  "total": 15
}
```

**Use Case:** When a user asks about a rejected show, admins can search by the show title or artist name to find the rejection reason and explain it to the user.

## Testing

### Local Development Testing

1. Create a test Discord server or use a `#dev-alerts` channel
2. Create a webhook for the test channel
3. Add to your `.env.development` file:
   ```bash
   DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
   DISCORD_NOTIFICATIONS_ENABLED=true
   ```
4. Start the backend server
5. Test each notification type:

#### Test New User Signup
```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com", "password": "testpass123"}'
```
Expected: Green "New User Registration" embed in Discord

#### Test New Show Submission
```bash
# First login to get auth cookie
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com", "password": "testpass123"}' \
  -c cookies.txt

# Submit a show
curl -X POST http://localhost:8080/shows \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "event_date": "2025-02-15T20:00:00Z",
    "city": "Phoenix",
    "state": "AZ",
    "venues": [{"name": "Test Venue", "city": "Phoenix", "state": "AZ"}],
    "artists": [{"name": "Test Band", "is_headliner": true}]
  }'
```
Expected: Blue "New Show" embed in Discord

#### Test Show Approval (requires admin)
```bash
# Login as admin and approve a pending show
curl -X POST http://localhost:8080/admin/shows/1/approve \
  -H "Content-Type: application/json" \
  -b admin-cookies.txt \
  -d '{"verify_venues": false}'
```
Expected: Green "Show Approved" embed in Discord

#### Test Show Rejection (requires admin)
```bash
curl -X POST http://localhost:8080/admin/shows/2/reject \
  -H "Content-Type: application/json" \
  -b admin-cookies.txt \
  -d '{"reason": "Duplicate show - already in calendar"}'
```
Expected: Red "Show Rejected" embed with reason in Discord

### Verifying Disabled State

1. Set `DISCORD_NOTIFICATIONS_ENABLED=false` (or remove the variable)
2. Redeploy (Coolify) or restart the server (local)
3. Perform any of the above actions
4. Verify no Discord messages are sent
5. Check logs to ensure no errors related to Discord

### Stage and Production Webhooks

Each environment should have its own webhook pointing to a dedicated channel:

| Environment | Channel | Purpose |
|-------------|---------|---------|
| Stage | `#stage-alerts` | Test notifications, catch issues before production |
| Production | `#production-alerts` | Real user activity monitoring |

**Recommended channel settings:**
- Restrict channel access to admins only
- Mute notifications or set to "Only @mentions" to avoid excessive pings
- Keep channels private to protect user privacy (emails are masked but still visible)

## Code Structure

### Files

| File | Purpose |
|------|---------|
| `internal/config/config.go` | Discord configuration struct and env loading |
| `internal/services/discord.go` | Discord service with all notification methods |
| `internal/api/handlers/auth.go` | Calls `NotifyNewUser` on registration |
| `internal/api/handlers/show.go` | Calls `NotifyNewShow` and `NotifyShowStatusChange` |
| `internal/api/handlers/admin.go` | Calls `NotifyShowApproved` and `NotifyShowRejected` |

### Service Methods

```go
// Check if Discord is configured
func (s *DiscordService) IsConfigured() bool

// Send notification for new user registration
func (s *DiscordService) NotifyNewUser(user *models.User)

// Send notification for new show submission
func (s *DiscordService) NotifyNewShow(show *ShowResponse, submitterEmail string)

// Send notification for show status changes
func (s *DiscordService) NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string)

// Send notification for show approval
func (s *DiscordService) NotifyShowApproved(show *ShowResponse)

// Send notification for show rejection
func (s *DiscordService) NotifyShowRejected(show *ShowResponse, reason string)
```

## Troubleshooting

### Notifications Not Appearing

1. **Check environment variables in Coolify**: Ensure both `DISCORD_WEBHOOK_URL` and `DISCORD_NOTIFICATIONS_ENABLED=true` are set in the application's environment variables
2. **Verify the deployment**: After adding/changing environment variables, ensure Coolify has redeployed the application
3. **Check webhook URL**: Verify the URL is correct and the webhook hasn't been deleted in Discord
4. **Check container logs in Coolify**: Look for `[Discord]` prefixed log messages indicating errors
5. **Test webhook directly**:
   ```bash
   curl -X POST "YOUR_WEBHOOK_URL" \
     -H "Content-Type: application/json" \
     -d '{"content": "Test message"}'
   ```

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `Failed to send webhook` | Network issue or invalid URL | Verify URL and network connectivity |
| `Webhook returned non-2xx status: 404` | Webhook deleted | Create a new webhook |
| `Webhook returned non-2xx status: 429` | Rate limited | Discord limits webhooks to 30 requests/minute |

### Rate Limits

Discord webhooks are rate-limited to approximately 30 requests per minute per webhook. For high-traffic scenarios:
- Consider batching notifications
- Use separate webhooks for different event types
- Implement exponential backoff (not currently implemented as notifications are informational)
