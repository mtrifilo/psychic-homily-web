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
- **Single webhook**: Different embed colors differentiate event types
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

### Environment Variables

Add these to your `.env` file or environment:

```bash
# Discord webhook URL from your Discord server
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN

# Enable/disable notifications (default: false)
DISCORD_NOTIFICATIONS_ENABLED=true
```

### Creating a Discord Webhook

1. Open your Discord server
2. Go to **Server Settings** → **Integrations** → **Webhooks**
3. Click **New Webhook**
4. Configure the webhook:
   - **Name**: e.g., "Psychic Homily Alerts"
   - **Channel**: Select the channel for notifications
   - **Avatar**: Optionally upload a custom icon
5. Click **Copy Webhook URL**
6. Add the URL to your environment variables

### Behavior When Not Configured

If `DISCORD_NOTIFICATIONS_ENABLED` is `false` or `DISCORD_WEBHOOK_URL` is empty:
- All notification calls become no-ops
- No errors are thrown
- No logs are generated for skipped notifications
- API responses are unaffected

## Notification Content

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
```

### Show Status Change

```
Title: "Show Status Changed: Band Name at Venue"
Description: approved → pending
Color: Orange
Fields:
  - Show ID: 456
  - Changed By: us***@example.com
```

### Show Approved

```
Title: "Show Approved: Band Name at Venue"
Color: Green
Fields:
  - Show ID: 456
  - Event Date: Jan 15, 2025
  - Venue(s): The Rebel Lounge
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
```

## Testing

### Local Development Testing

1. Create a test Discord server or use a private channel in an existing server
2. Create a webhook for the test channel
3. Set environment variables:
   ```bash
   export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..."
   export DISCORD_NOTIFICATIONS_ENABLED=true
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

1. Set `DISCORD_NOTIFICATIONS_ENABLED=false`
2. Restart the server
3. Perform any of the above actions
4. Verify no Discord messages are sent
5. Check logs to ensure no errors related to Discord

### Production Webhook

For production, create a separate webhook in your production Discord server:
- Use a dedicated `#alerts` or `#admin-notifications` channel
- Restrict channel access to admins only
- Consider setting up Discord notification settings to avoid excessive pings

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

1. **Check environment variables**: Ensure both `DISCORD_WEBHOOK_URL` and `DISCORD_NOTIFICATIONS_ENABLED=true` are set
2. **Check webhook URL**: Verify the URL is correct and the webhook hasn't been deleted
3. **Check server logs**: Look for `[Discord]` prefixed log messages indicating errors
4. **Test webhook directly**:
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
