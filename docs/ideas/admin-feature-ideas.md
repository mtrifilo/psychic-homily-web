# Admin Feature Ideas

**Date:** 2026-01-31
**Status:** Planning

## Overview

This document outlines potential admin features for the Psychic Homily platform, organized by category with implementation considerations.

### Current Admin Features

| Feature | Description | Status |
|---------|-------------|--------|
| Venue Edits | Approve/reject user-submitted venue changes | Implemented |
| Unverified Venues | Verify new venues to display full addresses | Implemented |
| Import Show | Bulk import shows from markdown files | Implemented |
| Show Reports | Review user-submitted show issue reports (cancelled, sold out, inaccurate) | Implemented |
| Audit Log | Track all admin actions with timestamp, actor, and details | Implemented |
| User List/Search | View all users with search, auth methods, submission stats | Implemented |

---

## 1. User Management

Features for managing user accounts and permissions.

| Feature | Description | Priority | Status |
|---------|-------------|----------|--------|
| **User List/Search** | View all users with search by email, username, or registration date | High | ✅ Implemented |
| **User Detail View** | See user profile, submission history, saved shows, favorite venues | High | Not started |
| **Suspend Account** | Temporarily disable a user's ability to submit content | High | Not started |
| **Ban Account** | Permanently disable account with reason tracking | Medium | Not started |
| **Role Management** | Assign admin or moderator roles to trusted users | Medium | Not started |
| **Submission Stats** | View count of approved/rejected submissions per user | Low | ✅ Implemented (included in User List) |

### User List/Search (Implemented)

**Backend**:
- Service: `backend/internal/services/user.go` — `ListUsers()` with `AdminUserFilters`, batch-loaded passkey counts + show stats (avoids N+1)
- Handler: `backend/internal/api/handlers/admin.go` — `GetAdminUsersHandler`
- No new migrations (all data from existing tables)

**Frontend**:
- "Users" tab in Admin Console (`Users` icon, no badge count)
- `AdminUserCard` component: email, username, name, auth method badges (password/google/passkey), admin/deleted/inactive status badges, color-coded submission stats, join date
- `useAdminUsers` hook with debounced search, 30s stale time
- Search input with 300ms debounce, loading/error/empty states

**API Endpoints**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/users` | List users (paginated, searchable by email/username) |

### Future Implementation Notes
- Add `IsSuspended`, `SuspendedUntil`, `SuspendedReason` fields to User model
- Add user detail drawer/modal showing submission history
- Consider "moderator" role with limited admin powers (approve shows but not manage users)

---

## 2. Content Moderation

Features for handling reported or problematic content.

| Feature | Description | Priority | Status |
|---------|-------------|----------|--------|
| **Report System** | Allow users to flag shows as cancelled, sold out, or inaccurate | High | ✅ Implemented |
| **Report Queue** | Admin view of all pending reports with dismiss/resolve options | High | ✅ Implemented |
| **Artist Merge Tool** | Consolidate duplicate artist entries (e.g., "The National" vs "National, The") | Medium | Not started |
| **Venue Merge Tool** | Consolidate duplicate venues (same place, different names) | Medium | Not started |
| **Duplicate Detection** | Automated suggestions for potential duplicates based on name similarity | Low | Not started |
| **Bulk Delete** | Remove multiple spam submissions at once | Low | Not started |

### Show Report System (Implemented)

**Database**: `show_reports` table with:
- `report_type` enum: `cancelled`, `sold_out`, `inaccurate`
- `status` enum: `pending`, `dismissed`, `resolved`
- Unique constraint: one report per user per show
- Foreign keys to `shows` and `users`

**Backend**:
- Model: `backend/internal/models/show_report.go`
- Service: `backend/internal/services/show_report.go`
- Handler: `backend/internal/api/handlers/show_report.go`
- Discord notification on new report

**Frontend**:
- `ReportShowButton` on show detail page (authenticated users only)
- `ReportShowDialog` with report type selection and optional details
- Admin "Reports" tab with pending count badge
- `ShowReportCard` with dismiss/resolve actions

**API Endpoints**:
| Method | Path | Description |
|--------|------|-------------|
| POST | `/shows/{id}/report` | Submit a report |
| GET | `/shows/{id}/my-report` | Check if user has reported |
| GET | `/admin/reports` | List pending reports |
| POST | `/admin/reports/{id}/dismiss` | Mark as spam/invalid |
| POST | `/admin/reports/{id}/resolve` | Mark as action taken |

### Future Enhancements
- Extend to venue/artist reports
- Merge tools need careful handling of relationships (shows linked to merged artists)
- Consider soft-merge that keeps old slug as redirect

---

## 3. Analytics & Insights

Features for understanding platform activity and trends.

| Feature | Description | Priority |
|---------|-------------|----------|
| **Dashboard Overview** | Key metrics at a glance: pending items, recent activity, totals | High |
| **Submission Trends** | Chart of shows submitted per week/month | Medium |
| **Popular Venues** | Venues ranked by show count or user favorites | Medium |
| **Active Users** | Users ranked by submission count or engagement | Medium |
| **Verification Metrics** | Average time from venue creation to verification | Low |
| **Geographic Heatmap** | Shows/venues by city visualization | Low |

### Implementation Notes
- Start with simple counts (total shows, venues, users, pending items)
- Use existing data - no new models needed for basic analytics
- Consider caching aggregated stats to avoid expensive queries
- Chart library: Recharts or Chart.js (already common in React ecosystems)

---

## 4. Operations & Tools

Features for day-to-day admin operations.

| Feature | Description | Priority | Status |
|---------|-------------|----------|--------|
| **Audit Log** | Track all admin actions with timestamp, user, and details | High | ✅ Implemented |
| **Bulk Venue Verify** | Verify multiple venues at once | Medium | Not started |
| **Export Data** | CSV export of shows, venues, or users for external analysis | Medium | Not started |
| **Notification Settings** | Configure Discord webhook URLs and alert thresholds | Low | Not started |
| **System Health** | View API response times, error rates, background job status | Low | Not started |
| **Feature Flags** | Toggle features on/off without deploys | Low | Not started |

### Audit Log (Implemented)

**Database**: `audit_logs` table (migration 000022) with:
- `actor_id` FK to users, `action` VARCHAR(50), `entity_type` VARCHAR(50), `entity_id` INT
- `metadata` JSONB for action-specific context (rejection reasons, notes, flags)
- Indexes on `created_at DESC`, `(entity_type, entity_id)`, `actor_id`
- Immutable, append-only (no `updated_at`)

**Backend**:
- Model: `backend/internal/models/audit_log.go`
- Service: `backend/internal/services/audit_log.go` — fire-and-forget `LogAction`, paginated `GetAuditLogs`
- Handler: `backend/internal/api/handlers/audit_log.go`
- Instrumented in `admin.go` (5 actions) and `show_report.go` (3 actions)

**8 Actions Instrumented**:
| Action | Entity Type | Handler |
|--------|-------------|---------|
| `approve_show` | `show` | `admin.go` |
| `reject_show` | `show` | `admin.go` |
| `verify_venue` | `venue` | `admin.go` |
| `approve_venue_edit` | `venue_edit` | `admin.go` |
| `reject_venue_edit` | `venue_edit` | `admin.go` |
| `dismiss_report` | `show_report` | `show_report.go` |
| `resolve_report` | `show_report` | `show_report.go` |
| `resolve_report_with_flag` | `show_report` | `show_report.go` |

**Frontend**:
- "Audit Log" tab in Admin Console (`ScrollText` icon, no badge count)
- `AuditLogEntry` component with action icons, color-coding, actor email, timestamp
- `useAuditLogs` hook with 30s stale time

**API Endpoints**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/audit-logs` | List audit logs (paginated, filterable by `entity_type`, `action`) |

### Other Implementation Notes
- Export can start simple: server-side CSV generation, download link
- Feature flags could use environment variables initially, then graduate to DB-backed

---

## 5. Communication

Features for admin-to-user communication.

| Feature | Description | Priority |
|---------|-------------|----------|
| **Rejection Messages** | Customizable rejection reason templates | Medium |
| **User Notifications** | In-app notifications for submission status changes | Medium |
| **Announcement Banner** | Site-wide message for maintenance or updates | Low |
| **Email Templates** | Manage email content for notifications | Low |

### Implementation Notes
- Rejection templates: Store common reasons, allow selection + custom text
- In-app notifications: `Notification` model with `UserID, Type, Message, ReadAt, CreatedAt`
- Banner: Simple config value, displayed on all pages when set

---

## Recommended Implementation Order

### Phase 1: Foundation
1. ~~Audit Log~~ ✅ Implemented (8 admin actions instrumented, admin UI tab)
2. ~~User List/Search~~ ✅ Implemented (search, auth methods, submission stats per user)
3. Dashboard Overview (quick health check)

### Phase 2: Moderation
4. ~~Report System + Queue~~ ✅ Implemented (show reports)
5. User Suspend/Ban
6. Bulk Actions

### Phase 3: Data Quality
7. Artist Merge Tool
8. Venue Merge Tool
9. Duplicate Detection

### Phase 4: Insights
10. Analytics Charts
11. Export Data
12. Advanced Metrics

---

## Questions to Consider

1. **Moderator Role** - Do you want a middle tier between regular users and admins?
2. **Report Threshold** - Should content be auto-hidden after N reports?
3. **Appeal Process** - Should rejected submissions have an appeal workflow?
4. **Data Retention** - How long to keep audit logs and deleted content?
