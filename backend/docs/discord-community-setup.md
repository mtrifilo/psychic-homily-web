# Discord Community Setup Guide

A high-level checklist for setting up the Psychic Homily Discord server, including admin notifications and community channels.

> **Technical details**: For webhook configuration, notification formats, and troubleshooting, see [discord-notifications.md](./discord-notifications.md).

---

## 1. Server Setup Checklist

- [ ] Create a new Discord server named "Psychic Homily" (or similar)
- [ ] Upload server icon (use Psychic Homily branding)
- [ ] Set server description in **Server Settings → Overview**
- [ ] Enable Community features if desired (**Server Settings → Enable Community**)

### Recommended Category Structure

```
ADMIN (private)
├── #admin-alerts
├── #admin-chat
└── #mod-log

GENERAL (public)
├── #welcome
├── #rules
├── #announcements
└── #general-chat

SHOWS & MUSIC (public)
├── #upcoming-shows
├── #show-discussion
├── #artist-spotlight
└── #venue-talk

FEEDBACK (public)
├── #feature-requests
├── #bug-reports
└── #site-feedback
```

---

## 2. Admin Channels (Private)

### Create Admin Category

- [ ] Create category: `ADMIN`
- [ ] Set category permissions: **@everyone** → Deny "View Channel"
- [ ] Create role: `@Admin` with administrator permissions
- [ ] Grant `@Admin` access to the ADMIN category

### Admin Channels

| Channel | Purpose |
|---------|---------|
| `#admin-alerts` | Webhook notifications (signups, submissions, approvals) |
| `#admin-chat` | Private discussion between admins |
| `#mod-log` | Audit log for moderation actions (optional) |

### Configure #admin-alerts

- [ ] Create the channel under ADMIN category
- [ ] Set channel topic: "Automated alerts from Psychic Homily"
- [ ] Consider muting @everyone notifications (members configure their own)
- [ ] Pin a message explaining what notifications appear here

---

## 3. Community Channels (Public)

### Suggested Channels

| Channel | Purpose |
|---------|---------|
| `#welcome` | Auto-welcome new members, intro to the server |
| `#rules` | Community guidelines and code of conduct |
| `#announcements` | Site updates, new features, important news |
| `#general-chat` | Off-topic community conversation |
| `#upcoming-shows` | Discussion about shows on the calendar |
| `#show-discussion` | Post-show recaps, reviews, photos |
| `#artist-spotlight` | Share and discover local artists |
| `#venue-talk` | Venue recommendations and reviews |
| `#feature-requests` | Community suggestions for the site |
| `#bug-reports` | User-reported issues |

### Basic Moderation Notes

- [ ] Create `@Moderator` role with appropriate permissions
- [ ] Enable slow mode on high-traffic channels if needed (5-10 seconds)
- [ ] Set up AutoMod for spam/link filtering (**Server Settings → Safety Setup**)
- [ ] Consider requiring phone/email verification for new members

---

## 4. Webhook Connection

### Create the Webhook

1. Go to `#admin-alerts` → **Edit Channel** → **Integrations** → **Webhooks**
2. Click **New Webhook**
3. Configure:
   - **Name**: `Psychic Homily`
   - **Avatar**: Upload app logo (optional)
4. Click **Copy Webhook URL**

### Update Environment Variables

| Environment | Action |
|-------------|--------|
| **Local** | Add to `.env` file |
| **Staging** | Update in deployment config |
| **Production** | Update in production secrets |

```bash
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
DISCORD_NOTIFICATIONS_ENABLED=true
```

### Verify Connection

After deploying, trigger a test event (e.g., create a test user) and confirm the notification appears in `#admin-alerts`.

---

## 5. Community Invitation

### Invite Link Configuration

1. Go to **Server Settings → Invites**
2. Create invite with settings:
   - **Expire After**: Never
   - **Max Uses**: No limit
   - **Temporary Membership**: Disabled
3. Copy the invite link

### Where to Share

- [ ] Add to site footer or "Community" page
- [ ] Include in welcome emails to new users
- [ ] Add to social media bios

### Onboarding Considerations

- [ ] Set up a welcome message in `#welcome` explaining the community
- [ ] Create a `#rules` channel with clear community guidelines
- [ ] Consider a "Member" role granted after agreeing to rules (Discord Onboarding feature)
- [ ] Pin helpful links (site URL, how to submit shows, etc.)

---

## 6. Future Ideas

### Regional Scaling

As the app expands to venues across the US, consider restructuring channels:

- **Regional categories**: Create city/region groupings (`PHOENIX`, `LOS ANGELES`, etc.) with dedicated `-shows` and `-venues` channels
- **Forum channels**: Use Discord's forum feature for threaded discussions by city or venue, reducing channel sprawl
- **Role-based pings**: Let users self-assign region roles (`@Phoenix`, `@LA`) to receive only relevant announcements
- **Automated regional posts**: Future bot could route show announcements to the appropriate regional channel based on venue location

### Bot Enhancements

- **Show announcements**: Auto-post approved shows to `#upcoming-shows`
- **Daily/weekly digest**: Summary of newly added shows
- **Artist notifications**: Alert when specific artists are added
- **Calendar sync**: Remind users of shows they're interested in

### Community Growth Features

- **Role-based access**: Venue owners, verified artists, power users
- **Event threads**: Auto-create discussion threads for each show
- **Integration with site**: Link Discord accounts to Psychic Homily profiles
- **Giveaways/contests**: Ticket giveaways for community engagement

### Moderation Tools

- **Verification bot**: Require users to verify via the main site
- **Reputation system**: Track helpful community members
- **Ticket system**: Private support channels for user issues

---

## Quick Reference

| Task | Location |
|------|----------|
| Create webhook | Channel Settings → Integrations → Webhooks |
| Set permissions | Server Settings → Roles |
| Enable Community | Server Settings → Enable Community |
| AutoMod setup | Server Settings → Safety Setup |
| Invite settings | Server Settings → Invites |

---

## Related Documentation

- [Discord Notifications (Technical)](./discord-notifications.md) - Webhook implementation, notification formats, and troubleshooting
