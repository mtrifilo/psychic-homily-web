'use client'

import { useState, useMemo, useCallback, useEffect } from 'react'
import {
  Loader2,
  Inbox,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  History,
  Plus,
  PlusCircle,
  X,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  AdminEmptyState,
  CategoryBadge,
  RejectWithReasonRow,
  NotesActionRow,
} from '@/components/admin'
import { UserAttribution } from '@/components/shared'
import {
  useAdminPendingEdits,
  useApprovePendingEdit,
  useRejectPendingEdit,
} from '@/lib/hooks/admin/useAdminPendingEdits'
import {
  useAdminEntityReports,
  useResolveEntityReport,
  useDismissEntityReport,
  useAdminHideCollection,
} from '@/lib/hooks/admin/useAdminEntityReports'
import {
  useAdminPendingComments,
  useAdminApproveComment,
  useAdminRejectComment,
  useAdminHideComment,
} from '@/lib/hooks/admin/useAdminComments'
import {
  useAdminEntityRequests,
  useDecideEntityRequest,
  type ShowArtistInput,
  type ShowVenueInput,
} from '@/lib/hooks/admin/useAdminEntityRequests'
import { CommentEditHistory } from '@/features/comments'
import { EntitySaveSuccessBanner } from '@/features/contributions'
import type { PendingEditResponse } from '@/lib/hooks/admin/useAdminPendingEdits'
import type { EntityReportResponse } from '@/lib/hooks/admin/useAdminEntityReports'
import type { PendingComment } from '@/lib/hooks/admin/useAdminComments'
import type { AdminEntityRequest } from '@/lib/hooks/admin/useAdminEntityRequests'

// ─── Helpers ─────────────────────────────────────────────────────────────────

function getEntityUrl(entityType: string, entityId: number, entitySlug?: string): string {
  switch (entityType) {
    case 'artist':
      return `/artists/${entityId}`
    case 'venue':
      return `/venues/${entityId}`
    case 'festival':
      return `/festivals/${entityId}`
    case 'show':
      return `/shows/${entityId}`
    case 'comment':
      return '#'
    // PSY-357: collections are addressed by slug, not numeric ID. Fall back
    // to '#' if the slug couldn't be resolved (deleted collection, etc.).
    case 'collection':
      return entitySlug ? `/collections/${entitySlug}` : '#'
    // PSY-661: releases are addressed by slug. The backend resolves the slug
    // onto report.entity_slug; fall back to '#' if it couldn't be resolved.
    case 'release':
      return entitySlug ? `/releases/${entitySlug}` : '#'
    // PSY-666: labels are addressed by slug. The backend resolves the slug
    // onto report.entity_slug; fall back to '#' if it couldn't be resolved.
    case 'label':
      return entitySlug ? `/labels/${entitySlug}` : '#'
    default:
      return '#'
  }
}

function entityTypeLabel(entityType: string): string {
  return entityType.charAt(0).toUpperCase() + entityType.slice(1)
}

function reportTypeLabel(reportType: string): string {
  return reportType
    .split('_')
    .map(w => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
}

function timeAgo(dateStr: string): string {
  const now = new Date()
  const date = new Date(dateStr)
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  return `${months}mo ago`
}

function renderValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '(empty)'
  return String(value)
}

// ─── Filter Types ────────────────────────────────────────────────────────────

type ItemTypeFilter = 'all' | 'edits' | 'reports' | 'comments' | 'requests'
type EntityTypeFilter = '' | 'artist' | 'venue' | 'festival' | 'show' | 'collection' | 'release' | 'label'

// ─── Unified Item Type ───────────────────────────────────────────────────────

type ModerationItem =
  | { type: 'edit'; data: PendingEditResponse }
  | { type: 'report'; data: EntityReportResponse }
  | { type: 'comment'; data: PendingComment }
  | { type: 'request'; data: AdminEntityRequest }

// ─── PSY-603: success banner state ───────────────────────────────────────────

type ModerationActionVerb = 'approved' | 'rejected' | 'created'

interface ModerationAction {
  verb: ModerationActionVerb
  entityLabel: string
}

const SUCCESS_BANNER_TIMEOUT_MS = 5000

// ─── Pending Edit Card ───────────────────────────────────────────────────────

function PendingEditCard({
  edit,
  onActionSuccess,
}: {
  edit: PendingEditResponse
  onActionSuccess: (action: ModerationAction) => void
}) {
  const [expanded, setExpanded] = useState(false)

  const approveMutation = useApprovePendingEdit()
  const rejectMutation = useRejectPendingEdit()

  const isActioning = approveMutation.isPending || rejectMutation.isPending

  const entityLabel = edit.entity_name || `${entityTypeLabel(edit.entity_type)} #${edit.entity_id}`

  const handleApprove = useCallback(() => {
    approveMutation.mutate(edit.id, {
      // PSY-603: bubble success up to ModerationQueue so the page-level
      // banner can render. The card itself is about to unmount because the
      // pending-edits query gets invalidated, so a card-local banner would
      // disappear with the row.
      onSuccess: () => onActionSuccess({ verb: 'approved', entityLabel }),
    })
  }, [approveMutation, edit.id, onActionSuccess, entityLabel])

  const handleReject = useCallback(
    (reason: string) => {
      rejectMutation.mutate(
        { editId: edit.id, reason },
        {
          onSuccess: () => onActionSuccess({ verb: 'rejected', entityLabel }),
        }
      )
    },
    [rejectMutation, edit.id, onActionSuccess, entityLabel]
  )

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="edit" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(edit.entity_type)}
            </Badge>
            <a
              href={getEntityUrl(edit.entity_type, edit.entity_id)}
              className="text-sm font-medium text-foreground hover:underline truncate"
              target="_blank"
              rel="noopener noreferrer"
            >
              {entityLabel}
              <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
            </a>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(edit.created_at)}
          </span>
        </div>

        <div className="mt-2 text-sm text-muted-foreground">
          <span>
            by{' '}
            <UserAttribution
              name={edit.submitter_name}
              username={edit.submitter_username}
            />
          </span>
          {edit.summary && (
            edit.summary_html ? (
              <span
                className="ml-1 prose prose-sm max-w-none inline [&>p]:inline [&>p]:m-0"
                dangerouslySetInnerHTML={{ __html: `&mdash; ${edit.summary_html}` }}
              />
            ) : (
              <span className="ml-1">
                &mdash; {edit.summary}
              </span>
            )
          )}
        </div>

        {/* Changes preview / expand */}
        <button
          onClick={() => setExpanded(!expanded)}
          className="mt-2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
          {edit.field_changes.length} field change{edit.field_changes.length !== 1 ? 's' : ''}
        </button>

        {expanded && (
          <div className="mt-2 space-y-1.5 rounded-md border bg-muted/30 p-3 text-sm">
            {edit.field_changes.map((change, idx) => (
              <div key={idx} className="space-y-0.5">
                <span className="font-medium text-muted-foreground">{change.field}:</span>
                <div className="flex gap-2 flex-wrap text-xs font-mono">
                  <span className="bg-red-500/10 text-red-700 dark:text-red-400 rounded px-1.5 py-0.5 line-through">
                    {renderValue(change.old_value)}
                  </span>
                  <span className="text-muted-foreground">&rarr;</span>
                  <span className="bg-green-500/10 text-green-700 dark:text-green-400 rounded px-1.5 py-0.5">
                    {renderValue(change.new_value)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Approve-immediate + reject-with-required-reason (PSY-920 Model A) */}
        <RejectWithReasonRow
          onApprove={handleApprove}
          onReject={handleReject}
          isActioning={isActioning}
          isApproving={approveMutation.isPending}
          isRejecting={rejectMutation.isPending}
          rejectPlaceholder="Rejection reason (required) -- be specific to help the contributor learn"
        />

        {/* Error display */}
        {(approveMutation.isError || rejectMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(approveMutation.error || rejectMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Entity Request Card (PSY-871) ───────────────────────────────────────────

/** Display label for a request: the payload's name/title, else a type fallback. */
function requestEntityLabel(req: AdminEntityRequest): string {
  const p = req.payload || {}
  const name = p.name ?? p.title
  if (typeof name === 'string' && name.trim()) return name
  return `${entityTypeLabel(req.entity_type)} request`
}

function sourceContextLabel(source: string): string {
  switch (source) {
    case 'ai_extraction':
      return 'AI extraction'
    case 'paste_mode':
      return 'paste'
    case 'manual':
      return 'manual'
    default:
      return source
  }
}

// name/title are surfaced as the card header (requestEntityLabel), so the
// preview omits them to avoid repeating the label — mirroring PendingEditCard,
// whose preview shows the changes, not the already-headed entity name.
const PREVIEW_OMIT_KEYS = new Set(['name', 'title'])

/** Non-header payload fields as [key, displayValue] pairs for the preview box. */
function payloadPreviewEntries(payload: Record<string, unknown>): Array<[string, string]> {
  return Object.entries(payload || {})
    .filter(([k, v]) => !PREVIEW_OMIT_KEYS.has(k) && v !== null && v !== undefined && v !== '')
    .map(
      ([k, v]) => [k, typeof v === 'object' ? JSON.stringify(v) : String(v)] as [string, string]
    )
}

// Entity types whose request payloads the backend can fulfill on approve
// (PSY-1008; festival added in PSY-998 — series_slug is derived from the name;
// show added in PSY-1037 — the card collects the venue + artist associations
// the payload lacks before approving). All current types are fulfillable; the
// set + the disabled-Create hint below stay as the guard for any future type
// that lands without a fulfillment branch. MUST stay in sync with the backend
// fulfillEntity switch (entity_request_fulfill.go) — enabling a type here
// before its backend branch lands would claim-then-422 the request.
const FULFILLABLE_REQUEST_TYPES = new Set([
  'artist',
  'venue',
  'label',
  'release',
  'festival',
  'show',
])

// One artist row in the show-create form (PSY-1037).
interface ShowArtistRow {
  name: string
  isHeadliner: boolean
}

/**
 * Inline associations form for approving a SHOW request (PSY-1037): the
 * payload carries the show metadata but not the venue + artists CreateShow
 * requires, so the admin supplies them here. Plain controlled inputs — the
 * backend find-or-creates venues by name+city+state and artists by name
 * (admin-created venues are auto-verified), so no autocomplete is needed.
 * Typo-created duplicates are recoverable via the existing merge tooling.
 */
function ShowCreateForm({
  defaultCity,
  defaultState,
  isSubmitting,
  onSubmit,
  onCancel,
}: {
  defaultCity: string
  defaultState: string
  isSubmitting: boolean
  onSubmit: (venue: ShowVenueInput, artists: ShowArtistInput[]) => void
  onCancel: () => void
}) {
  const [venueName, setVenueName] = useState('')
  const [venueCity, setVenueCity] = useState(defaultCity)
  const [venueState, setVenueState] = useState(defaultState)
  // First artist defaults to headliner (mirrors the backend's first-artist
  // fallback, but explicit so the admin sees + can change it).
  const [artists, setArtists] = useState<ShowArtistRow[]>([{ name: '', isHeadliner: true }])

  const updateArtist = (index: number, patch: Partial<ShowArtistRow>) => {
    setArtists(rows => rows.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  const filledArtists = artists.filter(a => a.name.trim() !== '')
  const canSubmit =
    venueName.trim() !== '' &&
    venueCity.trim() !== '' &&
    venueState.trim() !== '' &&
    filledArtists.length > 0 &&
    !isSubmitting

  const inputClass =
    'w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring'

  return (
    <div className="mt-3 space-y-3 rounded-md border bg-muted/30 p-3">
      <p className="text-xs font-medium text-foreground">
        Create show — supply the venue and artist(s) the request doesn&rsquo;t carry
      </p>

      <div className="space-y-2">
        <input
          value={venueName}
          onChange={e => setVenueName(e.target.value)}
          placeholder="Venue name"
          aria-label="Venue name"
          className={inputClass}
          disabled={isSubmitting}
        />
        <div className="flex gap-2">
          <input
            value={venueCity}
            onChange={e => setVenueCity(e.target.value)}
            placeholder="City"
            aria-label="Venue city"
            className={inputClass}
            disabled={isSubmitting}
          />
          <input
            value={venueState}
            onChange={e => setVenueState(e.target.value)}
            placeholder="State"
            aria-label="Venue state"
            maxLength={10} // venues.state is VARCHAR(10)
            className={`${inputClass} max-w-24`}
            disabled={isSubmitting}
          />
        </div>
      </div>

      <div className="space-y-2">
        {artists.map((artist, index) => (
          <div key={index} className="flex items-center gap-2">
            <input
              value={artist.name}
              onChange={e => updateArtist(index, { name: e.target.value })}
              placeholder={`Artist ${index + 1} name`}
              aria-label={`Artist ${index + 1} name`}
              className={inputClass}
              disabled={isSubmitting}
            />
            <label className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={artist.isHeadliner}
                onChange={e => updateArtist(index, { isHeadliner: e.target.checked })}
                aria-label={`Artist ${index + 1} is headliner`}
                disabled={isSubmitting}
              />
              headliner
            </label>
            {artists.length > 1 && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setArtists(rows => rows.filter((_, i) => i !== index))}
                aria-label={`Remove artist ${index + 1}`}
                disabled={isSubmitting}
              >
                <X className="h-3 w-3" />
              </Button>
            )}
          </div>
        ))}
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setArtists(rows => [...rows, { name: '', isHeadliner: false }])}
          disabled={isSubmitting}
        >
          <Plus className="h-3 w-3 mr-1" />
          Add artist
        </Button>
      </div>

      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          disabled={!canSubmit}
          onClick={() =>
            onSubmit(
              {
                name: venueName.trim(),
                city: venueCity.trim(),
                state: venueState.trim(),
              },
              filledArtists.map(a => ({
                name: a.name.trim(),
                is_headliner: a.isHeadliner,
              }))
            )
          }
        >
          {isSubmitting ? (
            <Loader2 className="h-3 w-3 animate-spin mr-1" />
          ) : (
            <PlusCircle className="h-3 w-3 mr-1" />
          )}
          Create show
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onCancel} disabled={isSubmitting}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

/** Returns url only when it's a safe http(s) link, else undefined (no link). */
function safeHttpUrl(url: string | undefined): string | undefined {
  return url && /^https?:\/\//i.test(url) ? url : undefined
}

/**
 * The 4th moderation card type: a queued entity-CREATION request. Mirrors
 * PendingEditCard's structure (meta row → attribution/source → preview →
 * action row) so admins keep one scan path. "Create" approves the request →
 * the backend creates the catalog entity (PSY-1008); "Reject" expands the
 * shared required-reason textarea. No entity link in the header — the entity
 * does not exist yet.
 */
function RequestCard({
  request,
  onActionSuccess,
}: {
  request: AdminEntityRequest
  onActionSuccess: (action: ModerationAction) => void
}) {
  const decideMutation = useDecideEntityRequest()
  const isActioning = decideMutation.isPending
  // One mutation drives both actions; key each spinner off which decision is in
  // flight so only the active button spins (the two-mutation cards get this for
  // free; here we read the mutation's in-flight variables).
  const pendingDecision = isActioning ? decideMutation.variables?.decision : undefined

  const entityLabel = requestEntityLabel(request)
  const previewEntries = payloadPreviewEntries(request.payload)
  const sourceUrl = safeHttpUrl(request.source_detail?.url)
  const canCreate = FULFILLABLE_REQUEST_TYPES.has(request.entity_type)
  // PSY-1037: a show approve needs admin-supplied venue + artists, so Create
  // opens the associations form instead of approving immediately.
  const isShow = request.entity_type === 'show'
  const [showFormOpen, setShowFormOpen] = useState(false)

  const handleCreate = useCallback(() => {
    if (isShow) {
      // Open-only: the form's own Cancel closes it (a "Create" button that
      // toggles closed reads as a broken submit).
      setShowFormOpen(true)
      return
    }
    decideMutation.mutate(
      { id: request.id, decision: 'approved' },
      { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
    )
  }, [isShow, decideMutation, request.id, onActionSuccess, entityLabel])

  const handleCreateShow = useCallback(
    (venue: ShowVenueInput, artists: ShowArtistInput[]) => {
      decideMutation.mutate(
        { id: request.id, decision: 'approved', show_venue: venue, show_artists: artists },
        { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
      )
    },
    [decideMutation, request.id, onActionSuccess, entityLabel]
  )

  const handleReject = useCallback(
    (reason: string) => {
      decideMutation.mutate(
        { id: request.id, decision: 'rejected', note: reason },
        { onSuccess: () => onActionSuccess({ verb: 'rejected', entityLabel }) }
      )
    },
    [decideMutation, request.id, onActionSuccess, entityLabel]
  )

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4">
        {/* Header row — no entity link: the entity does not exist yet */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="request" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(request.entity_type)}
            </Badge>
            <span className="text-sm font-medium text-foreground truncate">
              {entityLabel}
            </span>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(request.created_at)}
          </span>
        </div>

        {/* Attribution + source context */}
        <div className="mt-2 text-sm text-muted-foreground">
          <span>
            by{' '}
            <UserAttribution
              name={request.requester_name}
              username={request.requester_username}
            />
          </span>
          <span className="ml-1">
            &middot; via {sourceContextLabel(request.source_context)}
          </span>
          {sourceUrl && (
            <a
              href={sourceUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="ml-1 inline-flex items-center gap-0.5 hover:text-foreground hover:underline"
            >
              source
              <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>

        {/* Source excerpt (AI-extracted requests) */}
        {request.source_detail?.excerpt && (
          <p className="mt-1 text-xs italic text-muted-foreground line-clamp-2">
            &ldquo;{request.source_detail.excerpt}&rdquo;
          </p>
        )}

        {/* Payload preview — key:value monospace in a muted box. Always shown
            (the request payload is small; the action is inline, not
            expand-to-detail per the locked design). */}
        {previewEntries.length > 0 && (
          <div className="mt-2 space-y-0.5 rounded-md border bg-muted/30 p-3 text-xs font-mono">
            {previewEntries.map(([key, value]) => (
              <div key={key} className="flex gap-2">
                <span className="text-muted-foreground">{key}:</span>
                <span className="text-foreground break-all">{value}</span>
              </div>
            ))}
          </div>
        )}

        {/* Unreachable for the current types (all fulfillable as of PSY-1037);
            kept as the guard for a future entity type that lands without a
            fulfillment branch. */}
        {!canCreate && (
          <p className="mt-2 text-xs text-muted-foreground">
            {entityTypeLabel(request.entity_type)} requests must be created
            manually for now — Create isn&rsquo;t supported for this type yet.
          </p>
        )}

        {/* PSY-1037: show approvals collect the venue + artists here first */}
        {isShow && showFormOpen && (
          <ShowCreateForm
            defaultCity={typeof request.payload?.city === 'string' ? request.payload.city : ''}
            defaultState={typeof request.payload?.state === 'string' ? request.payload.state : ''}
            isSubmitting={pendingDecision === 'approved'}
            onSubmit={handleCreateShow}
            onCancel={() => setShowFormOpen(false)}
          />
        )}

        {/* Create-immediate + reject-with-required-reason (same model as edits) */}
        <RejectWithReasonRow
          onApprove={handleCreate}
          onReject={handleReject}
          isActioning={isActioning}
          isApproving={pendingDecision === 'approved'}
          isRejecting={pendingDecision === 'rejected'}
          approveLabel="Create"
          approveIcon={PlusCircle}
          approveDisabled={!canCreate || (isShow && showFormOpen)}
          rejectPlaceholder="Rejection reason (required) -- tell the requester why"
        />

        {decideMutation.isError && (
          <p className="mt-2 text-xs text-destructive">
            {decideMutation.error?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Entity Report Card ──────────────────────────────────────────────────────

function EntityReportCard({ report }: { report: EntityReportResponse }) {
  const resolveMutation = useResolveEntityReport()
  const dismissMutation = useDismissEntityReport()

  const isActioning = resolveMutation.isPending || dismissMutation.isPending

  const handleConfirm = useCallback(
    (actionKey: string, notes: string) => {
      const mutation = actionKey === 'resolve' ? resolveMutation : dismissMutation
      mutation.mutate({ reportId: report.id, notes: notes || undefined })
    },
    [resolveMutation, dismissMutation, report.id]
  )

  return (
    <Card className="overflow-hidden">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="report" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(report.entity_type)}
            </Badge>
            <a
              href={getEntityUrl(report.entity_type, report.entity_id, report.entity_slug)}
              className="text-sm font-medium text-foreground hover:underline truncate"
              target="_blank"
              rel="noopener noreferrer"
            >
              {report.entity_name || `${entityTypeLabel(report.entity_type)} #${report.entity_id}`}
              <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
            </a>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        {/* Meta */}
        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={report.reporter_username}
              />
            </span>
          </div>
          {report.details && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{report.details}&rdquo;
            </p>
          )}
        </div>

        {/* Dual-action + optional-notes (PSY-920 Model B) */}
        <NotesActionRow
          actions={[
            {
              key: 'resolve',
              restingLabel: 'Resolve',
              confirmLabel: 'Confirm Resolve',
              variant: 'default',
              icon: 'check',
              notesPlaceholder: 'Admin notes (optional) -- describe the action taken',
            },
            {
              key: 'dismiss',
              restingLabel: 'Dismiss',
              confirmLabel: 'Confirm Dismiss',
              variant: 'outline',
              icon: 'x',
              notesPlaceholder: 'Admin notes (optional) -- explain why this was dismissed',
            },
          ]}
          onConfirm={handleConfirm}
          isActioning={isActioning}
        />

        {/* Error display */}
        {(resolveMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(resolveMutation.error || dismissMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Pending Comment Card ───────────────────────────────────────────────────

function PendingCommentCard({ comment }: { comment: PendingComment }) {
  // PSY-297: edit history viewer, opened on demand
  const [isEditHistoryOpen, setIsEditHistoryOpen] = useState(false)

  const approveMutation = useAdminApproveComment()
  const rejectMutation = useAdminRejectComment()

  const isActioning = approveMutation.isPending || rejectMutation.isPending

  const handleApprove = useCallback(() => {
    approveMutation.mutate(comment.id)
  }, [approveMutation, comment.id])

  const handleReject = useCallback(
    (reason: string) => {
      rejectMutation.mutate({ commentId: comment.id, reason })
    },
    [rejectMutation, comment.id]
  )

  const entityUrl = getEntityUrl(comment.entity_type, comment.entity_id)
  const editCount = comment.edit_count ?? 0
  const hasEdits = editCount > 0

  return (
    <Card className="overflow-hidden" data-testid="pending-comment-card">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="comment" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(comment.entity_type)}
            </Badge>
            {comment.entity_name && (
              <a
                href={entityUrl}
                className="text-sm font-medium text-foreground hover:underline truncate"
                target="_blank"
                rel="noopener noreferrer"
              >
                {comment.entity_name}
                <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
              </a>
            )}
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(comment.created_at)}
          </span>
        </div>

        <div className="mt-2 text-sm text-muted-foreground flex items-center flex-wrap gap-2">
          <span>
            by{' '}
            <UserAttribution
              name={comment.author_name}
              username={comment.author_username}
            />
          </span>
          {comment.trust_tier && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              {comment.trust_tier}
            </Badge>
          )}
          {/* PSY-297: edit count badge + click-to-view-history.
              Only rendered when the comment has at least one recorded edit. */}
          {hasEdits && (
            <button
              type="button"
              onClick={() => setIsEditHistoryOpen(true)}
              className="inline-flex items-center gap-1 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-400 hover:bg-amber-500/20 transition-colors"
              data-testid="pending-comment-edit-badge"
              aria-label={`View edit history (${editCount} edit${editCount !== 1 ? 's' : ''})`}
            >
              <History className="h-3 w-3" />
              {editCount} edit{editCount !== 1 ? 's' : ''}
            </button>
          )}
        </div>

        {/* Comment body */}
        <div
          className="mt-2 rounded-md border bg-muted/30 p-3 text-sm prose prose-sm dark:prose-invert max-w-none"
          dangerouslySetInnerHTML={{ __html: comment.body_html }}
          data-testid="comment-body"
        />

        {/* PSY-297: edit history dialog, mounted on demand. */}
        {isEditHistoryOpen && (
          <CommentEditHistory
            open={isEditHistoryOpen}
            onOpenChange={setIsEditHistoryOpen}
            commentId={comment.id}
          />
        )}

        {/* Approve-immediate + reject-with-required-reason (PSY-920 Model A) */}
        <RejectWithReasonRow
          onApprove={handleApprove}
          onReject={handleReject}
          isActioning={isActioning}
          isApproving={approveMutation.isPending}
          isRejecting={rejectMutation.isPending}
          rejectPlaceholder="Rejection reason (required)"
        />

        {/* Error display */}
        {(approveMutation.isError || rejectMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(approveMutation.error || rejectMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Comment Report Card ────────────────────────────────────────────────────

function CommentReportCard({ report }: { report: EntityReportResponse }) {
  const hideMutation = useAdminHideComment()
  const dismissMutation = useDismissEntityReport()

  const isActioning = hideMutation.isPending || dismissMutation.isPending

  const handleConfirm = useCallback(
    (actionKey: string, notes: string) => {
      if (actionKey === 'hide') {
        hideMutation.mutate({
          commentId: report.entity_id,
          reason: notes || 'Hidden via report review',
        })
      } else {
        dismissMutation.mutate({ reportId: report.id, notes: notes || undefined })
      }
    },
    [hideMutation, dismissMutation, report.id, report.entity_id]
  )

  // Truncate comment body for preview
  const bodyPreview = report.details
    ? (report.details.length > 200 ? report.details.substring(0, 200) + '...' : report.details)
    : undefined

  return (
    <Card className="overflow-hidden" data-testid="comment-report-card">
      <CardContent className="p-4">
        {/* Header row */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="report" />
            <Badge variant="outline" className="shrink-0">
              Comment
            </Badge>
            <span className="text-sm text-muted-foreground truncate">
              {report.entity_name || `Comment #${report.entity_id}`}
            </span>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        {/* Meta */}
        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={report.reporter_username}
              />
            </span>
          </div>
          {bodyPreview && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{bodyPreview}&rdquo;
            </p>
          )}
        </div>

        {/* Dual-action + optional-notes (PSY-920 Model B) */}
        <NotesActionRow
          actions={[
            {
              key: 'hide',
              restingLabel: 'Hide Comment',
              confirmLabel: 'Confirm Hide',
              variant: 'destructive',
              icon: 'x',
              notesPlaceholder: 'Reason for hiding (optional)',
            },
            {
              key: 'dismiss',
              restingLabel: 'Dismiss Report',
              confirmLabel: 'Confirm Dismiss',
              variant: 'outline',
              icon: 'check',
              notesPlaceholder: 'Notes for dismissal (optional)',
            },
          ]}
          onConfirm={handleConfirm}
          isActioning={isActioning}
        />

        {/* Error display */}
        {(hideMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(hideMutation.error || dismissMutation.error)?.message || 'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Collection Report Card ────────────────────────────────────────────────

/**
 * PSY-357: admin moderation card for collection reports. Mirrors
 * `CommentReportCard` — a single click both hides the collection from
 * public browse (PUT /collections/{slug} with is_public=false) AND marks
 * the report resolved. The "Dismiss" path leaves the collection alone and
 * just clears the report.
 *
 * Hide is unavailable when the slug couldn't be resolved (i.e. the
 * collection was deleted between report submission and review). In that
 * case the only useful action is Dismiss.
 */
function CollectionReportCard({ report }: { report: EntityReportResponse }) {
  const hideMutation = useAdminHideCollection()
  const resolveMutation = useResolveEntityReport()
  const dismissMutation = useDismissEntityReport()

  const isActioning =
    hideMutation.isPending || resolveMutation.isPending || dismissMutation.isPending

  const entityUrl = getEntityUrl(report.entity_type, report.entity_id, report.entity_slug)
  const hasSlug = Boolean(report.entity_slug)

  const handleConfirm = useCallback(
    (actionKey: string, notes: string) => {
      if (actionKey === 'hide') {
        if (!report.entity_slug) return
        // Hide first, then resolve the report so the moderation queue
        // reflects the action taken (rather than two separate concerns).
        hideMutation.mutate(
          { slug: report.entity_slug },
          {
            onSuccess: () => {
              resolveMutation.mutate({ reportId: report.id, notes: notes || undefined })
            },
          }
        )
      } else {
        dismissMutation.mutate({ reportId: report.id, notes: notes || undefined })
      }
    },
    [hideMutation, resolveMutation, dismissMutation, report.id, report.entity_slug]
  )

  return (
    <Card className="overflow-hidden" data-testid="collection-report-card">
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="report" />
            <Badge variant="outline" className="shrink-0">
              Collection
            </Badge>
            {hasSlug ? (
              <a
                href={entityUrl}
                className="text-sm font-medium text-foreground hover:underline truncate"
                target="_blank"
                rel="noopener noreferrer"
              >
                {report.entity_name || `Collection #${report.entity_id}`}
                <ExternalLink className="h-3 w-3 inline ml-1 opacity-50" />
              </a>
            ) : (
              <span className="text-sm font-medium text-muted-foreground truncate">
                {report.entity_name || `Collection #${report.entity_id}`} (deleted)
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(report.created_at)}
          </span>
        </div>

        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <Badge variant="outline" className="text-xs">
              {reportTypeLabel(report.report_type)}
            </Badge>
            <span className="text-muted-foreground">
              by{' '}
              <UserAttribution
                name={report.reporter_name}
                username={report.reporter_username}
              />
            </span>
          </div>
          {report.details && (
            <p className="text-sm text-muted-foreground italic">
              &ldquo;{report.details}&rdquo;
            </p>
          )}
        </div>

        {/* Dual-action + optional-notes (PSY-920 Model B). Hide is disabled
            when the collection was deleted (no slug); the only useful action
            then is Dismiss. */}
        <NotesActionRow
          actions={[
            {
              key: 'hide',
              restingLabel: 'Hide from Public Browse',
              confirmLabel: 'Confirm Hide',
              variant: 'destructive',
              icon: 'x',
              notesPlaceholder: 'Reason for hiding from public browse (optional)',
              disabled: !hasSlug,
              title: hasSlug ? undefined : 'Cannot hide — collection was deleted',
            },
            {
              key: 'dismiss',
              restingLabel: 'Dismiss Report',
              confirmLabel: 'Confirm Dismiss',
              variant: 'outline',
              icon: 'check',
              notesPlaceholder: 'Notes for dismissal (optional)',
            },
          ]}
          onConfirm={handleConfirm}
          isActioning={isActioning}
        />

        {(hideMutation.isError || resolveMutation.isError || dismissMutation.isError) && (
          <p className="mt-2 text-xs text-destructive">
            {(hideMutation.error || resolveMutation.error || dismissMutation.error)?.message ||
              'Action failed'}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Main Component ──────────────────────────────────────────────────────────

export function ModerationQueue() {
  const [itemTypeFilter, setItemTypeFilter] = useState<ItemTypeFilter>('all')
  const [entityTypeFilter, setEntityTypeFilter] = useState<EntityTypeFilter>('')

  // PSY-603: page-level success banner. Cards bubble up via onActionSuccess
  // because they unmount on success (the row is removed from the queue).
  // Auto-dismisses after SUCCESS_BANNER_TIMEOUT_MS, and clears immediately
  // when the admin changes either filter (treating filter change as a "tab
  // change" — a fresh review surface should not carry a stale confirmation).
  const [lastAction, setLastAction] = useState<ModerationAction | null>(null)

  const handleActionSuccess = useCallback((action: ModerationAction) => {
    setLastAction(action)
  }, [])

  useEffect(() => {
    if (!lastAction) return
    const timer = setTimeout(() => setLastAction(null), SUCCESS_BANNER_TIMEOUT_MS)
    return () => clearTimeout(timer)
  }, [lastAction])

  // Clear the stale success banner when either filter changes (treating a
  // filter change as a "tab change" — a fresh review surface should not carry
  // a stale confirmation). React 19.2: adjust state during render via the
  // canonical previous-value-guard idiom instead of a cascading effect.
  const [prevFilterKey, setPrevFilterKey] = useState(
    `${itemTypeFilter}|${entityTypeFilter}`
  )
  const filterKey = `${itemTypeFilter}|${entityTypeFilter}`
  if (filterKey !== prevFilterKey) {
    setPrevFilterKey(filterKey)
    setLastAction(null)
  }

  // Fetch pending edits
  const {
    data: editsData,
    isLoading: editsLoading,
    error: editsError,
  } = useAdminPendingEdits({
    status: 'pending',
    entity_type: entityTypeFilter || undefined,
  })

  // Fetch pending entity reports
  const {
    data: reportsData,
    isLoading: reportsLoading,
    error: reportsError,
  } = useAdminEntityReports({
    status: 'pending',
    entity_type: entityTypeFilter || undefined,
  })

  // Fetch pending comments
  const {
    data: commentsData,
    isLoading: commentsLoading,
    error: commentsError,
  } = useAdminPendingComments()

  // Fetch pending entity-creation requests (PSY-871). Shares the entity_type
  // filter; source_context is left unfiltered (the queue shows all origins).
  const {
    data: requestsData,
    isLoading: requestsLoading,
    error: requestsError,
  } = useAdminEntityRequests({
    state: 'pending',
    entity_type: entityTypeFilter || undefined,
  })

  const isLoading = editsLoading || reportsLoading || commentsLoading || requestsLoading
  const error = editsError || reportsError || commentsError || requestsError

  // Merge and sort items by created_at (oldest first for review fairness)
  const items = useMemo<ModerationItem[]>(() => {
    const editItems: ModerationItem[] = (editsData?.edits || []).map(e => ({
      type: 'edit' as const,
      data: e,
    }))
    // All reports (entity + comment reports) are of type 'report' in the unified list
    const reportItems: ModerationItem[] = (reportsData?.reports || []).map(r => ({
      type: 'report' as const,
      data: r,
    }))
    const commentItems: ModerationItem[] = (commentsData?.comments || []).map(c => ({
      type: 'comment' as const,
      data: c,
    }))
    const requestItems: ModerationItem[] = (requestsData?.requests || []).map(r => ({
      type: 'request' as const,
      data: r,
    }))

    let merged = [...editItems, ...reportItems, ...commentItems, ...requestItems]

    // Apply item type filter
    if (itemTypeFilter === 'edits') {
      merged = merged.filter(i => i.type === 'edit')
    } else if (itemTypeFilter === 'reports') {
      merged = merged.filter(i => i.type === 'report')
    } else if (itemTypeFilter === 'comments') {
      merged = merged.filter(i => i.type === 'comment')
    } else if (itemTypeFilter === 'requests') {
      merged = merged.filter(i => i.type === 'request')
    }

    // Sort oldest first (review fairness)
    merged.sort(
      (a, b) =>
        new Date(a.data.created_at).getTime() - new Date(b.data.created_at).getTime()
    )

    return merged
  }, [editsData, reportsData, commentsData, requestsData, itemTypeFilter])

  const totalEdits = editsData?.total || 0
  const totalReports = reportsData?.total || 0
  const totalComments = commentsData?.total || 0
  const totalRequests = requestsData?.total || 0
  const totalItems = totalEdits + totalReports + totalComments + totalRequests

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
        <p className="text-destructive">
          {error instanceof Error
            ? error.message
            : 'Failed to load moderation queue. Please try again.'}
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* PSY-603 / PSY-622: page-level success banner. Reuses the shared
          EntitySaveSuccessBanner primitive (originally PSY-562's "Changes
          saved" entity-detail banner) with an action-specific message. */}
      {lastAction && (
        <EntitySaveSuccessBanner
          visible
          message={formatModerationActionMessage(lastAction)}
        />
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Item type filter */}
        <div className="flex items-center gap-1 rounded-lg border bg-muted/30 p-0.5">
          <FilterButton
            active={itemTypeFilter === 'all'}
            onClick={() => setItemTypeFilter('all')}
            label="All"
            count={totalItems}
          />
          <FilterButton
            active={itemTypeFilter === 'edits'}
            onClick={() => setItemTypeFilter('edits')}
            label="Edits"
            count={totalEdits}
          />
          <FilterButton
            active={itemTypeFilter === 'reports'}
            onClick={() => setItemTypeFilter('reports')}
            label="Reports"
            count={totalReports}
          />
          <FilterButton
            active={itemTypeFilter === 'comments'}
            onClick={() => setItemTypeFilter('comments')}
            label="Comments"
            count={totalComments}
          />
          <FilterButton
            active={itemTypeFilter === 'requests'}
            onClick={() => setItemTypeFilter('requests')}
            label="Requests"
            count={totalRequests}
          />
        </div>

        {/* Entity type filter */}
        <select
          value={entityTypeFilter}
          onChange={e => setEntityTypeFilter(e.target.value as EntityTypeFilter)}
          className="rounded-md border bg-background px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
        >
          <option value="">All entity types</option>
          <option value="artist">Artists</option>
          <option value="venue">Venues</option>
          <option value="festival">Festivals</option>
          <option value="show">Shows</option>
          <option value="collection">Collections</option>
          <option value="release">Releases</option>
          <option value="label">Labels</option>
        </select>

        {/* Summary count */}
        <span className="text-sm text-muted-foreground ml-auto">
          {items.length} item{items.length !== 1 ? 's' : ''} pending review
        </span>
      </div>

      {/* Empty state */}
      {items.length === 0 && (
        <AdminEmptyState
          icon={Inbox}
          title="Queue Clear"
          message={
            itemTypeFilter === 'edits'
              ? 'No pending entity edits to review.'
              : itemTypeFilter === 'reports'
                ? 'No pending entity reports to review.'
                : itemTypeFilter === 'comments'
                  ? 'No pending comments to review.'
                  : itemTypeFilter === 'requests'
                    ? 'No pending entity-creation requests to review.'
                    : 'No items need moderation. Pending entity edits, reports, comments, and creation requests will appear here when users submit them.'
          }
        />
      )}

      {/* Items list */}
      {items.length > 0 && (
        <div className="grid gap-3">
          {items.map(item => {
            if (item.type === 'edit') {
              return (
                <PendingEditCard
                  key={`edit-${item.data.id}`}
                  edit={item.data as PendingEditResponse}
                  onActionSuccess={handleActionSuccess}
                />
              )
            }
            if (item.type === 'comment') {
              return <PendingCommentCard key={`comment-${item.data.id}`} comment={item.data as PendingComment} />
            }
            if (item.type === 'request') {
              return (
                <RequestCard
                  key={`request-${item.data.id}`}
                  request={item.data as AdminEntityRequest}
                  onActionSuccess={handleActionSuccess}
                />
              )
            }
            // Reports — type-specific cards for kinds that need bespoke
            // moderation actions (hide-comment, hide-collection); generic
            // EntityReportCard for the other entity types.
            const report = item.data as EntityReportResponse
            if (report.entity_type === 'comment') {
              return <CommentReportCard key={`comment-report-${report.id}`} report={report} />
            }
            if (report.entity_type === 'collection') {
              return <CollectionReportCard key={`collection-report-${report.id}`} report={report} />
            }
            return <EntityReportCard key={`report-${report.id}`} report={report} />
          })}
        </div>
      )}
    </div>
  )
}

// ─── Moderation Success Banner (PSY-603 / PSY-622) ───────────────────────────

/**
 * Maps a successful Approve/Reject action onto the message string passed to
 * the shared {@link EntitySaveSuccessBanner}. Approve names the affected
 * entity so admins can confirm they actioned the right row at a glance;
 * Reject leans on the rejection-reason input as the source of truth and
 * just confirms the submitter was notified.
 *
 * Originally an inline {@link ModerationSuccessBanner} (PSY-603); collapsed
 * to a string formatter in PSY-622 once {@link EntitySaveSuccessBanner} grew
 * an optional `message` prop.
 */
function formatModerationActionMessage(action: ModerationAction): string {
  switch (action.verb) {
    case 'created':
      return `Created — ${action.entityLabel} added to the catalog`
    case 'approved':
      return `Approved — change applied to ${action.entityLabel}`
    default:
      return 'Rejected — submitter notified of reason'
  }
}

// ─── Filter Button ───────────────────────────────────────────────────────────

function FilterButton({
  active,
  onClick,
  label,
  count,
}: {
  active: boolean
  onClick: () => void
  label: string
  count: number
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-md px-3 py-1 text-sm font-medium transition-colors ${
        active
          ? 'bg-background text-foreground shadow-sm'
          : 'text-muted-foreground hover:text-foreground'
      }`}
    >
      {label}
      {count > 0 && (
        <span className={`ml-1.5 text-xs ${active ? 'text-muted-foreground' : 'opacity-70'}`}>
          {count}
        </span>
      )}
    </button>
  )
}
