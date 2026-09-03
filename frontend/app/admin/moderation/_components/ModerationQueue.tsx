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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
  useRescueEntityRequest,
  type ShowArtistInput,
  type ShowVenueInput,
} from '@/lib/hooks/admin/useAdminEntityRequests'
import { isConflictError } from '@/lib/api'
import { CommentEditHistory } from '@/features/comments'
import { EntitySaveSuccessBanner } from '@/features/contributions'
// Imported by path, not through the '@/features/shows' barrel, which is
// root-layout reachable (see features/sharedChunkBarrelGuard.test.ts). Note the
// barrel was never actually an option here: it does not re-export
// show-form-utils at all, so the path import is the only way to reach the
// vocabulary.
//
// It is NOT free, and the honest number is worth writing down: show-form-utils
// value-imports lib/utils/timeUtils, lib/utils/formatters and
// features/shows/utils, so roughly a thousand lines of date and form-mapping
// code ride into the admin chunk for two constants. Tolerated because the
// alternative is duplicating the vocabulary, which is the thing that must not
// drift. The durable fix is to split the vocabulary into its own leaf module
// that both this form and ShowForm import; that is a features/shows change and
// is deliberately not made here.
import {
  DEFAULT_SET_TYPE,
  SET_TYPE_OPTIONS,
  SET_TYPE_VALUES,
} from '@/features/shows/components/show-form-utils'
import type { SetType } from '@/features/shows/types'
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

/**
 * How far updated_at must lead created_at before a pending request reads as
 * revised. One minute, by product decision.
 *
 * It is a floor, not a correction for clock skew: an untouched row's two
 * stamps are EQUAL, because GORM writes every auto-timestamp on a row from one
 * clock read and the column defaults are both NOW(). So the cost of the floor
 * is its own blind spot, a resubmission inside the first minute never badging,
 * and not a false-positive risk it is buying that back.
 */
const REVISED_MIN_GAP_MS = 60 * 1000

/**
 * True when a PENDING entity request was rewritten after it was filed: a
 * resubmission replaces the queued row's payload in place, moving updated_at
 * while created_at stays where the queue sorts on it.
 *
 * PENDING is the caller's contract. On a decided row the same gap only means
 * the decision was stamped.
 */
function wasRevisedSinceFiling(createdAt: string, updatedAt: string): boolean {
  const created = Date.parse(createdAt)
  const updated = Date.parse(updatedAt)
  if (Number.isNaN(created) || Number.isNaN(updated)) return false
  return updated - created > REVISED_MIN_GAP_MS
}

// ─── Filter Types ────────────────────────────────────────────────────────────

// 'needs_attention' (PSY-1088) is the rescue view: approved-but-unfulfilled
// requests, NOT pending ones — a separate review surface from the other four.
type ItemTypeFilter =
  | 'all'
  | 'edits'
  | 'reports'
  | 'comments'
  | 'requests'
  | 'needs_attention'
  | 'withdrawn'
type EntityTypeFilter = '' | 'artist' | 'venue' | 'festival' | 'show' | 'collection' | 'release' | 'label'

// ─── Unified Item Type ───────────────────────────────────────────────────────

type ModerationItem =
  | { type: 'edit'; data: PendingEditResponse }
  | { type: 'report'; data: EntityReportResponse }
  | { type: 'comment'; data: PendingComment }
  | { type: 'request'; data: AdminEntityRequest }
  | { type: 'rescue'; data: AdminEntityRequest }
  | { type: 'withdrawn'; data: AdminEntityRequest }

// ─── PSY-603: success banner state ───────────────────────────────────────────

// 'voided' (PSY-1088) is the rescue-queue dismiss: distinct from 'rejected' so
// the banner doesn't claim the submitter was "notified" (a void dismisses an
// approved orphan; no notification is sent and the submitter saw it approved).
type ModerationActionVerb = 'approved' | 'rejected' | 'created' | 'voided'

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

  // `field_changes` is a nil-able Go slice, so the wire can send `null`: the
  // backend leaves it nil both when the column is NULL and when the stored
  // JSON fails to unmarshal. Reading `.length` off that threw (PSY-1600).
  const fieldChanges = edit.field_changes ?? []

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
          {fieldChanges.length} field change{fieldChanges.length !== 1 ? 's' : ''}
        </button>

        {expanded && (
          <div className="mt-2 space-y-1.5 rounded-md border bg-muted/30 p-3 text-sm">
            {fieldChanges.map((change, idx) => (
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

/**
 * Names the source_context a request was ORIGINALLY filed with, when a
 * resubmission has since replaced it with a different one (PSY-1978). Renders
 * nothing otherwise, so both cards can drop it into their source line
 * unconditionally.
 *
 * A resubmission overwrites payload, source_context and source_detail together,
 * so a request filed as `ai_extraction` with a source article and resubmitted as
 * `manual` presents as a plain manual request; this is what stops that being
 * silent. It stays silent when the value is unchanged, because "revised from
 * manual" beside "via manual" says nothing — a revision as such is not this
 * field's subject.
 *
 * Amber is this file's "worth knowing before you act" register. Unlike a
 * timestamp-derived revision signal, `original_source_context` does not move
 * when a row is decided, so it is equally truthful on an approved orphan.
 */
function RevisedFromSource({ request }: { request: AdminEntityRequest }) {
  const original = request.original_source_context
  if (!original || original === request.source_context) return null
  return (
    <span className="ml-1 text-amber-700 dark:text-amber-400">
      &middot; revised from {sourceContextLabel(original)}
    </span>
  )
}

// Surfaced as the card header (requestEntityLabel), so the preview does not
// repeat them — mirroring PendingEditCard, whose preview shows the changes and
// not the already-headed entity name.
const PREVIEW_HEADER_KEYS = ['name', 'title']

// Rendered by PayloadBillLine instead of the preview. `artists` is the show
// payload's bill, and it is the only non-scalar payload field, so the preview
// could only stringify it.
const PREVIEW_CONTROL_OWNED_KEYS = ['artists']

const PREVIEW_OMIT_KEYS = new Set([...PREVIEW_HEADER_KEYS, ...PREVIEW_CONTROL_OWNED_KEYS])

/** Non-header payload fields as [key, displayValue] pairs for the preview box. */
// `payload` is a *json.RawMessage server-side, so the wire can send `null`;
// the `|| {}` below is the guard, and the signature now says so (PSY-1600).
function payloadPreviewEntries(
  payload: Record<string, unknown> | null
): Array<[string, string]> {
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

/**
 * The moderation form's stand-in for "nobody has stated this act's slot"
 * (PSY-1856).
 *
 * It exists as a NAMED value rather than an empty string for two reasons that
 * point the same way: Radix's Select forbids an item whose value is `""`, and
 * `set_type: ""` is exactly what the API rejects — only an ABSENT key means the
 * slot is unknown (a present value must be in the vocabulary, so the empty
 * string 422s). Giving the unset state a value that is not a set_type makes the
 * rejected payload unrepresentable rather than merely avoided.
 */
const UNSTATED_ROLE = 'unstated'

/**
 * Compile-time proof that the sentinel is not itself a role. If the backend
 * ever adds a vocabulary member named 'unstated', this assignment stops
 * building. Without it the collision is silent and corrupting: the union
 * collapses, the Select renders two items sharing one value, and every act the
 * admin deliberately marked 'unstated' has its set_type OMITTED and is stored
 * as 'performer'. Mirrors show-form-utils.ts's UnlistedSetType guard.
 */
const _unstatedIsNotASetType: typeof UNSTATED_ROLE extends SetType ? never : true = true
void _unstatedIsNotASetType

type SetTypeChoice = SetType | typeof UNSTATED_ROLE

/**
 * The role choices offered per act: the unstated default first, then the whole
 * PSY-1673 vocabulary in the same presentation order the show form uses.
 *
 * "Role not stated" and "Performer (slot unknown)" are indistinguishable once
 * stored, BUT ONLY BECAUSE this form always sends an explicit `is_headliner`.
 * Given that, resolveArtistRole returns SetTypeDefault for an explicit
 * 'performer' and for an absent set_type alike, and nothing records which the
 * admin picked.
 *
 * Do not read that as "the two options are interchangeable at the endpoint".
 * They are not. buildShowAssociations flips billIsCurated on a present
 * set_type, and suppressPositionInference skips rows that state either field,
 * so for a payload that OMITTED is_headliner the two diverge: all-unstated
 * gives position 0 the headliner, all-'performer' gives the bill no headliner.
 * Making is_headliner conditional the way set_type already is would silently
 * activate that difference. toShowArtistInputs' always-send is the invariant
 * holding this equivalence up, and a test pins it.
 *
 * The unstated option is not redundant even so, because its whole job happens
 * BEFORE the write: it lets the form show a role nobody has chosen as unchosen,
 * instead of pre-filling a value the admin then has to notice and correct. The
 * honesty is in the control, not in the row.
 */
const BILL_ROLE_OPTIONS: ReadonlyArray<{ value: SetTypeChoice; label: string }> = [
  { value: UNSTATED_ROLE, label: 'Role not stated' },
  ...SET_TYPE_OPTIONS,
]

/**
 * Coerce a Select value back into the choice union.
 *
 * Unreachable in practice (Radix can only emit a value this file gave it), so
 * this is about which way to fail. The sibling `toSetType` is the wrong floor
 * here: it falls back to 'performer', and its own docs scope it to DISPLAY of
 * server-supplied values. On this WRITE path that fallback would turn a value
 * nobody recognised into a role the admin never chose, and any stated role is
 * what flips the whole bill to "curated" server-side. Falling back to unstated
 * says nothing instead, which is the only safe thing to say about a value we
 * could not read.
 */
function toSetTypeChoice(value: string): SetTypeChoice {
  const match = SET_TYPE_VALUES.find(setType => setType === value)
  return match ?? UNSTATED_ROLE
}

// One artist row in the show-create form (PSY-1037; bill role PSY-1856).
interface ShowArtistRow {
  name: string
  set_type: SetTypeChoice
}

/**
 * Map the form's artist rows onto the API payload (PSY-1856).
 *
 * Two rules, both deliberate:
 *
 *  1. `set_type` is OMITTED for an unstated row. The endpoint reads only an
 *     absent key as "slot unknown"; a present `""` is a 422. This is the same
 *     conditional-spread shape the decide/rescue mutations use for their own
 *     optional fields.
 *
 *  2. `is_headliner` is DERIVED from the role and always sent — never tracked
 *     beside it, exactly as ShowForm's toArtistPayloads does. Sending it is
 *     what keeps an entirely unstated bill honest: with no signal at all on any
 *     act, the backend falls back to reading position 0 as the headliner, which
 *     would re-introduce the guess the bill-role vocabulary exists to remove.
 *     An explicit `false` says "this act was not designated", so an unstated
 *     act stores `performer` whatever the rest of the bill says.
 */
function toShowArtistInputs(rows: ShowArtistRow[]): ShowArtistInput[] {
  return rows.map(row => ({
    name: row.name.trim(),
    is_headliner: row.set_type === 'headliner',
    ...(row.set_type === UNSTATED_ROLE ? {} : { set_type: row.set_type }),
  }))
}

/**
 * Read the bill a show request's payload carries (PSY-1858) into form rows.
 *
 * This is the ONLY place the bill's shape is asserted: `payload` is `unknown`
 * in the generated types, because the backend field is a *json.RawMessage and
 * no schema reaches the OpenAPI document. The authoritative shape is
 * communitym.ShowRequestArtist — a required `name`, an optional `set_type`
 * from the curated vocabulary.
 *
 * A non-conforming entry is DROPPED, never thrown on: this runs while
 * rendering the queue, so one malformed payload costs its own card a prefill
 * and not the whole surface. An empty result means "no bill", which the form
 * answers with a single blank row.
 *
 * The result is not truncated. MaxShowRequestArtists is enforced at
 * queue-create and again pre-claim; a cap restated here could only disagree
 * with it, and disagreeing downward would seed fewer acts than the request
 * holds while the admin submits the seeded rows as the whole bill.
 */
function parsePayloadBill(payload: Record<string, unknown> | null): ShowArtistRow[] {
  const artists = payload?.artists
  if (!Array.isArray(artists)) return []

  const rows: ShowArtistRow[] = []
  for (const entry of artists) {
    if (typeof entry !== 'object' || entry === null) continue
    const { name, set_type: setType } = entry as { name?: unknown; set_type?: unknown }
    if (typeof name !== 'string') continue
    // A nameless act is dropped from the submitted bill anyway, and one
    // carrying a role blocks the submit outright.
    const trimmedName = name.trim()
    if (trimmedName === '') continue
    rows.push({
      name: trimmedName,
      // An unrecognised role says nothing rather than asserting a slot nobody
      // chose; toSetTypeChoice falls back to unstated for exactly that reason.
      set_type: typeof setType === 'string' ? toSetTypeChoice(setType) : UNSTATED_ROLE,
    })
  }
  return rows
}

/**
 * Whether a row states a slot on the bill.
 *
 * "Curated" is the BACKEND's test, not the form's. `performer` is one of the
 * two spellings of "slot unknown" (headlineSlotUnknownValues), the other being
 * the absent key that parsePayloadBill reads as UNSTATED_ROLE, so a bill whose
 * acts are all explicitly "Performer (slot unknown)" is still uncurated and DOES
 * get a headline slot from position 0. Testing `!== UNSTATED_ROLE` alone would
 * read that bill as curated.
 *
 * One predicate for both readers of it: the card line, which annotates only a
 * stated role, and the form's partial-curation warning, which must match
 * headlineSlotSQL's notion of curated or it misfires.
 */
function curatesABillSlot(row: ShowArtistRow): row is ShowArtistRow & { set_type: SetType } {
  return row.set_type !== UNSTATED_ROLE && row.set_type !== DEFAULT_SET_TYPE
}

/**
 * How a stated bill role reads inside the compact card line.
 *
 * Lowercase, because this is a reading of the bill rather than a control: the
 * form's sentence-case option labels are a separate register and stay in
 * show-form-utils, which reserves annotation copy as its own decision.
 *
 * Typed as an exhaustive Record so a role added to the backend vocabulary stops
 * this file building until somebody decides how it reads here.
 */
const BILL_LINE_ROLE_LABELS: Record<SetType, string | null> = {
  headliner: 'headliner',
  direct_support: 'direct support',
  opener: 'opener',
  special_guest: 'special guest',
  dj: 'DJ',
  // Unreachable: curatesABillSlot rejects the neutral default before the lookup.
  performer: null,
}

/**
 * Acts printed in full on the compact line before the rest become a count.
 *
 * A scan affordance, not a cap on the bill: the count states exactly how many
 * acts it stands for, and the form still opens on every one of them.
 */
const BILL_LINE_MAX_ACTS = 5

/**
 * The bill a show request carries, as one line an admin can read without
 * opening the form.
 *
 * The reject and scan paths never open the form, so without this the bill is
 * invisible on them. It is a READING, never an editing surface.
 *
 * Renders nothing for a payload with no readable bill, which covers a bill-less
 * request and a malformed `artists` value alike: parsePayloadBill drops what it
 * cannot read, so a malformed payload costs its own card this line and not the
 * whole queue.
 *
 * Takes the whole request so the entity-type gate lives here rather than at
 * each card: a bill is a show's field, exactly as the backend's
 * ShowPayloadArtists answers nil for every other type.
 */
function PayloadBillLine({ request }: { request: AdminEntityRequest }) {
  if (request.entity_type !== 'show') return null
  const bill = parsePayloadBill(request.payload)
  if (bill.length === 0) return null

  const shown = bill.slice(0, BILL_LINE_MAX_ACTS)
  const overflow = bill.length - shown.length
  const acts = shown.map(row => {
    const label = curatesABillSlot(row) ? BILL_LINE_ROLE_LABELS[row.set_type] : null
    return label === null ? row.name : `${row.name} (${label})`
  })

  return (
    <p
      className="mt-1 flex items-baseline gap-1 text-sm text-muted-foreground"
      data-testid="moderation-bill-line"
    >
      {/* Names alone read as an unlabelled list out of visual context. */}
      <span className="sr-only">Bill: </span>
      {/* `truncate` keeps a long name from wrapping the card; the count is the
          separate question of acts past the fifth, and stays legible. */}
      <span className="min-w-0 truncate">{acts.join(' · ')}</span>
      {overflow > 0 && <span className="shrink-0">+{overflow}</span>}
    </p>
  )
}

/** A payload string field, or '' when the payload does not carry one. */
function payloadString(payload: Record<string, unknown> | null, key: string): string {
  const value = payload?.[key]
  return typeof value === 'string' ? value : ''
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
  payload,
  isSubmitting,
  onSubmit,
  onCancel,
}: {
  /**
   * The request's own creation payload. Every seed this form takes is read off
   * it here, so the approve card and the rescue card cannot drift apart on what
   * a request prefills.
   */
  payload: Record<string, unknown> | null
  isSubmitting: boolean
  onSubmit: (venue: ShowVenueInput, artists: ShowArtistInput[]) => void
  onCancel: () => void
}) {
  const [venueName, setVenueName] = useState('')
  const [venueCity, setVenueCity] = useState(() => payloadString(payload, 'city'))
  const [venueState, setVenueState] = useState(() => payloadString(payload, 'state'))
  // The bill the request carried WHEN THIS FORM OPENED. Read once, like every
  // other seed here: a queued payload is mutable (a resubmission replaces it in
  // place), and the queue refetches, so a live read would let the rows and
  // everything describing them disagree while the admin is typing.
  const [payloadBill] = useState(() => parsePayloadBill(payload))
  const seededFromPayload = payloadBill.length > 0
  // A seeded act keeps the role the payload stated and stays UNSTATED when it
  // stated none: bill order is not a designation, on this form or at the
  // endpoint. From here the rows are the admin's working copy; re-seeding them
  // from a refetched payload would discard edits mid-review.
  const [artists, setArtists] = useState<ShowArtistRow[]>(() =>
    payloadBill.length > 0 ? payloadBill : [{ name: '', set_type: UNSTATED_ROLE }]
  )

  // A seeded act is removable even when it is the only one: the bill came from
  // somebody else, possibly an AI extraction, so a single hallucinated act must
  // be droppable in one click. A form that seeded blank keeps its one-row floor,
  // where removal would only mean clearing a field the admin just typed into.
  const canRemoveRows = artists.length > 1 || seededFromPayload

  const updateArtist = (index: number, patch: Partial<ShowArtistRow>) => {
    setArtists(rows => rows.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  const filledArtists = artists.filter(a => a.name.trim() !== '')
  // A nameless row is dropped from the bill. That was harmless when the only
  // thing it carried was a checkbox, but it now carries the six-way role an
  // admin reaches for first, so dropping it silently discards a stated fact and
  // can leave the bill without the headliner somebody explicitly designated.
  // Block the submit instead: the admin either names the act or clears the role.
  const hasNamelessStatedRole = artists.some(
    a => a.name.trim() === '' && a.set_type !== UNSTATED_ROLE
  )

  // A bill somebody curated but gave no headliner has NO headline slot at all
  // in charts: headlineSlotSQL reads such a bill as curated and then takes the
  // headline slot to be exactly the rows marked 'headliner', inferring nothing
  // from list order. The show page meanwhile still renders the first act as the
  // headliner, so the two disagree. This form makes that state reachable in two
  // clicks, so it says so, and only says so. WARN, DO NOT BLOCK is the recorded
  // decision (PSY-1856): a bill with a stated opener and no stated headliner is
  // a legitimate description of a real bill, and the display reconciliation
  // belongs to PSY-1943's one-rule scope, not to a gate here.
  //
  // What counts as curated is curatesABillSlot's question, and the card's bill
  // line asks it too, so the two cannot disagree about what a stated role is.
  //
  // Read off filledArtists because that is the bill that gets sent: a nameless
  // row is dropped (and separately blocks the submit above).
  const hasCuratedBillWithoutHeadliner =
    filledArtists.some(curatesABillSlot) &&
    !filledArtists.some(row => row.set_type === 'headliner')

  const canSubmit =
    venueName.trim() !== '' &&
    venueCity.trim() !== '' &&
    venueState.trim() !== '' &&
    filledArtists.length > 0 &&
    !hasNamelessStatedRole &&
    !isSubmitting

  const inputClass =
    'w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring'

  return (
    <div className="mt-3 space-y-3 rounded-md border bg-muted/30 p-3">
      {/* A seeded form is not asking for artists the request lacks, so it does
          not say so. */}
      <p className="text-xs font-medium text-foreground">
        {seededFromPayload
          ? 'Create show — check the bill the requester recorded, then supply the venue'
          : 'Create show — supply the venue and artist(s) the request doesn’t carry'}
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
          // The row wraps below `sm`: the role Select is 224px of fixed width
          // where the headliner checkbox was ~70px, and an <input> will not
          // shrink past its intrinsic width, so on a phone the name field was
          // squeezed to a few characters. Above `sm` it is one row as before.
          <div key={index} className="flex flex-wrap items-center gap-2">
            <input
              value={artist.name}
              onChange={e => updateArtist(index, { name: e.target.value })}
              placeholder={`Artist ${index + 1} name`}
              aria-label={`Artist ${index + 1} name`}
              className={`${inputClass} min-w-0 flex-1 basis-full sm:basis-0`}
              disabled={isSubmitting}
            />
            {/* Bill role, replacing the headliner checkbox (PSY-1856):
                headliner is one role among six, and set_type outranks
                is_headliner server-side. Named by aria-label alone — this dense
                row has no visible label to conflict with (WCAG 2.5.3). */}
            <Select
              value={artist.set_type}
              onValueChange={value => updateArtist(index, { set_type: toSetTypeChoice(value) })}
              disabled={isSubmitting}
            >
              {/* Wide enough for the longest label, "Performer (slot unknown)".
                  The trigger line-clamps its value, so a narrower box renders
                  the neutral default as "Performer (slot unk". */}
              <SelectTrigger
                className="w-56 shrink-0"
                aria-label={`Artist ${index + 1} bill role`}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BILL_ROLE_OPTIONS.map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {canRemoveRows && (
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
          onClick={() => setArtists(rows => [...rows, { name: '', set_type: UNSTATED_ROLE }])}
          disabled={isSubmitting}
        >
          <Plus className="h-3 w-3 mr-1" />
          Add artist
        </Button>
      </div>

      {hasNamelessStatedRole && (
        <p className="text-xs text-muted-foreground">
          Name the act you gave a role to, or set its role back to “Role not stated”.
        </p>
      )}

      {/* Caution, not a gate: submission stays enabled. Amber is this file's
          established "worth knowing before you act" register (the rescue-request
          banner uses it), which is the distinction being drawn against the
          muted hint above, and it is legible in both themes. */}
      {hasCuratedBillWithoutHeadliner && (
        <p className="text-xs text-amber-700 dark:text-amber-400">
          No headliner stated. This bill will have no headline slot in charts.
        </p>
      )}

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
              toShowArtistInputs(filledArtists)
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
  // This card renders pending rows only, so a moved updated_at is a rewritten
  // submission and nothing else.
  const isRevised = wasRevisedSinceFiling(request.created_at, request.updated_at)
  // PSY-1974: the version the SHOW FORM was opened against, held for as long as
  // it is open.
  //
  // Every other action reads request.updated_at live, and should: the card
  // re-renders the payload it decides on, so live IS what the admin is looking
  // at. The form is the exception because its seeds are read once when it opens
  // — city, state and the bill (parsePayloadBill) — so a refetch landing while
  // the admin is typing leaves those rows describing a payload the row no longer
  // holds. Sending the live version there would let exactly that submission
  // through, which is the one case the version exists to refuse.
  const [showFormVersion, setShowFormVersion] = useState<string | null>(null)

  const handleCreate = useCallback(() => {
    if (isShow) {
      // Open-only: the form's own Cancel closes it (a "Create" button that
      // toggles closed reads as a broken submit).
      setShowFormVersion(request.updated_at)
      setShowFormOpen(true)
      return
    }
    decideMutation.mutate(
      { id: request.id, decision: 'approved', expected_updated_at: request.updated_at },
      { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
    )
  }, [
    isShow,
    setShowFormOpen,
    setShowFormVersion,
    decideMutation,
    request.id,
    request.updated_at,
    onActionSuccess,
    entityLabel,
  ])

  const handleCreateShow = useCallback(
    (venue: ShowVenueInput, artists: ShowArtistInput[]) => {
      decideMutation.mutate(
        {
          id: request.id,
          decision: 'approved',
          show_venue: venue,
          show_artists: artists,
          expected_updated_at: showFormVersion ?? request.updated_at,
        },
        { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
      )
    },
    [
      decideMutation,
      request.id,
      request.updated_at,
      showFormVersion,
      onActionSuccess,
      entityLabel,
    ]
  )

  const handleReject = useCallback(
    (reason: string) => {
      decideMutation.mutate(
        {
          id: request.id,
          decision: 'rejected',
          note: reason,
          expected_updated_at: request.updated_at,
        },
        { onSuccess: () => onActionSuccess({ verb: 'rejected', entityLabel }) }
      )
    },
    [decideMutation, request.id, request.updated_at, onActionSuccess, entityLabel]
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
            {/* Amber is this file's "worth knowing before you act" register
                (see the rescue card) and is legible in both themes. */}
            {isRevised && (
              <Badge
                variant="outline"
                className="shrink-0 border-amber-500/40 text-amber-700 dark:text-amber-400"
              >
                Revised
              </Badge>
            )}
            <span className="text-sm font-medium text-foreground truncate">
              {entityLabel}
            </span>
          </div>
          {/* created_at stays the headline stamp: it is what the queue sorts
              on, so the card keeps its place. */}
          <div className="flex flex-col items-end shrink-0 text-xs text-muted-foreground">
            <span>{timeAgo(request.created_at)}</span>
            {isRevised && <span>revised {timeAgo(request.updated_at)}</span>}
          </div>
        </div>

        {/* The bill, readable without opening the form */}
        <PayloadBillLine request={request} />

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
          <RevisedFromSource request={request} />
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
            payload={request.payload}
            isSubmitting={pendingDecision === 'approved'}
            onSubmit={handleCreateShow}
            onCancel={() => {
              setShowFormOpen(false)
              setShowFormVersion(null)
            }}
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
          <>
            <p className="mt-2 text-xs text-destructive">
              {decideMutation.error?.message || 'Action failed'}
            </p>
            {/* PSY-1974: a 409 means this client's view of the row is out of
                date, and the mutation answers every one of them by refetching
                the queue. It says the refetch is under way and nothing more:
                the three 409s the endpoint can return (revised, decided by
                someone else, catalog entity already exists) call for three
                different next steps, and only the server's own message above
                knows which one this is. */}
            {isConflictError(decideMutation.error) && (
              <p className="mt-1 text-xs text-muted-foreground">
                Refreshing the queue.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

// ─── Rescue Card (PSY-1088) ──────────────────────────────────────────────────

// ─── Withdrawn Request Card (PSY-1992) ───────────────────────────────────────

/**
 * A request its requester retracted while it was still pending.
 *
 * READ-ONLY, and that is the whole design: nothing here decides anything,
 * because a withdrawn row is not waiting on a moderator and every admin write
 * path is scoped to a state this row is not in. It exists so the queue can still
 * see what was asked for and by whom, which is what a withdrawal being a state
 * rather than a delete buys.
 *
 * It mirrors RequestCard's scan path (badge, type, name, attribution, payload
 * preview) so an admin reads a withdrawn row the same way they read a pending
 * one, minus the actions.
 */
function WithdrawnRequestCard({ request }: { request: AdminEntityRequest }) {
  const entityLabel = requestEntityLabel(request)
  const previewEntries = payloadPreviewEntries(request.payload)

  return (
    <Card className="overflow-hidden" data-testid="moderation-withdrawn-card">
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="request" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(request.entity_type)}
            </Badge>
            <Badge variant="outline" className="shrink-0 text-muted-foreground">
              Withdrawn
            </Badge>
            <span className="text-sm font-medium text-foreground truncate">
              {entityLabel}
            </span>
          </div>
          {/* Two moments, and they are different: when it was filed, and when
              the requester took it back. A card showing only the first cannot
              say how long ago the withdrawal happened. */}
          <div className="flex flex-col items-end shrink-0 text-xs text-muted-foreground">
            <span>filed {timeAgo(request.created_at)}</span>
            {request.decided_at && (
              <span>withdrawn {timeAgo(request.decided_at)}</span>
            )}
          </div>
        </div>

        {/* The bill: this card has no form at all, so it is the only reading */}
        <PayloadBillLine request={request} />

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
          <RevisedFromSource request={request} />
        </div>

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

        <p className="mt-2 text-xs text-muted-foreground">
          The requester withdrew this before it was reviewed. Nothing is waiting
          on you; it is kept for the record.
        </p>
      </CardContent>
    </Card>
  )
}

/**
 * A queued entity request that was APPROVED but whose catalog entity was never
 * created (created_entity_id IS NULL) — the "needs attention" rescue surface.
 * Two by-design routes lead here: a trusted-tier auto-approved SHOW (the
 * auto-approve path can't supply the venue + artist associations CreateShow
 * needs), and a post-claim fulfillment failure on the admin decide path.
 *
 * Mirrors RequestCard's scan path, but its primary action FULFILLS the orphan
 * (re-runs the catalog create — for a show, collecting the missing associations
 * via the shared ShowCreateForm first) instead of approving, and its secondary
 * action VOIDS it (rejects the orphan with a required reason). Both bypass the
 * decide flow, which only re-processes pending rows.
 */
function RescueCard({
  request,
  onActionSuccess,
}: {
  request: AdminEntityRequest
  onActionSuccess: (action: ModerationAction) => void
}) {
  const rescueMutation = useRescueEntityRequest()
  const isActioning = rescueMutation.isPending
  const pendingAction = isActioning ? rescueMutation.variables?.action : undefined

  const entityLabel = requestEntityLabel(request)
  const previewEntries = payloadPreviewEntries(request.payload)
  const canFulfill = FULFILLABLE_REQUEST_TYPES.has(request.entity_type)
  const isShow = request.entity_type === 'show'
  const [showFormOpen, setShowFormOpen] = useState(false)

  const handleFulfill = useCallback(() => {
    if (isShow) {
      // Open-only: the form's own Cancel closes it (mirrors RequestCard).
      setShowFormOpen(true)
      return
    }
    rescueMutation.mutate(
      { id: request.id, action: 'fulfill' },
      { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
    )
  }, [isShow, setShowFormOpen, rescueMutation, request.id, onActionSuccess, entityLabel])

  const handleFulfillShow = useCallback(
    (venue: ShowVenueInput, artists: ShowArtistInput[]) => {
      rescueMutation.mutate(
        { id: request.id, action: 'fulfill', show_venue: venue, show_artists: artists },
        { onSuccess: () => onActionSuccess({ verb: 'created', entityLabel }) }
      )
    },
    [rescueMutation, request.id, onActionSuccess, entityLabel]
  )

  const handleVoid = useCallback(
    (reason: string) => {
      rescueMutation.mutate(
        { id: request.id, action: 'void', note: reason },
        { onSuccess: () => onActionSuccess({ verb: 'voided', entityLabel }) }
      )
    },
    [rescueMutation, request.id, onActionSuccess, entityLabel]
  )

  return (
    <Card className="overflow-hidden border-amber-500/30">
      <CardContent className="p-4">
        {/* Header row — no entity link: the entity does not exist yet */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <CategoryBadge kind="rescue" />
            <Badge variant="outline" className="shrink-0">
              {entityTypeLabel(request.entity_type)}
            </Badge>
            <span className="text-sm font-medium text-foreground truncate">{entityLabel}</span>
          </div>
          <span className="text-xs text-muted-foreground shrink-0">
            {timeAgo(request.created_at)}
          </span>
        </div>

        {/* Why it's here */}
        <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
          Approved but never created — fulfill it or void it.
        </p>

        {/* The bill, readable without opening the form */}
        <PayloadBillLine request={request} />

        {/* Attribution */}
        <div className="mt-2 text-sm text-muted-foreground">
          <span>
            by{' '}
            <UserAttribution
              name={request.requester_name}
              username={request.requester_username}
            />
          </span>
          <span className="ml-1">&middot; via {sourceContextLabel(request.source_context)}</span>
          <RevisedFromSource request={request} />
        </div>

        {/* Payload preview */}
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

        {/* Guard for a future type without a fulfillment branch */}
        {!canFulfill && (
          <p className="mt-2 text-xs text-muted-foreground">
            {entityTypeLabel(request.entity_type)} requests must be created manually for now —
            Fulfill isn&rsquo;t supported for this type. Void it and create the entity directly.
          </p>
        )}

        {/* Shows collect the venue + artists before fulfilling */}
        {isShow && showFormOpen && (
          <ShowCreateForm
            payload={request.payload}
            isSubmitting={pendingAction === 'fulfill'}
            onSubmit={handleFulfillShow}
            onCancel={() => setShowFormOpen(false)}
          />
        )}

        {/* Fulfill-immediate + void-with-required-reason. The secondary action
            is a VOID (dismiss an approved orphan), not a reject — labeled so
            and given a void-specific success banner (no "submitter notified"). */}
        <RejectWithReasonRow
          onApprove={handleFulfill}
          onReject={handleVoid}
          isActioning={isActioning}
          isApproving={pendingAction === 'fulfill'}
          isRejecting={pendingAction === 'void'}
          approveLabel="Fulfill"
          approveIcon={PlusCircle}
          approveDisabled={!canFulfill || (isShow && showFormOpen)}
          rejectLabel="Void"
          rejectPlaceholder="Reason for voiding (required) -- why this approved request should be dismissed"
        />

        {rescueMutation.isError && (
          <p className="mt-2 text-xs text-destructive">
            {rescueMutation.error?.message || 'Action failed'}
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

  // The pending-comment card shows only the entity TYPE badge. `entity_name`
  // and `trust_tier` were declared on the old hand-written PendingComment but
  // the wire has never carried them (the endpoint returns the same
  // CommentResponse the public comment endpoints do), so the entity link and
  // trust-tier badge that read them were permanently invisible. Removed with
  // the type alias in PSY-1600; restoring either affordance needs the field
  // added to CommentResponse first.
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

  // PSY-1088: approved-but-unfulfilled rescue queue ("needs attention"). A
  // SEPARATE fetch from the pending queue above — these rows are approved, not
  // pending, and need fulfill/void rather than approve/reject. Always fetched
  // so the filter badge can show the count (consistent with the other four).
  const {
    data: rescueData,
    isLoading: rescueLoading,
    error: rescueError,
  } = useAdminEntityRequests({
    state: 'approved',
    unfulfilled: true,
    entity_type: entityTypeFilter || undefined,
  })

  // PSY-1992: requests their requester retracted while they were pending. A
  // SEPARATE fetch, and not part of any pending view: nobody is being asked to
  // decide these, and the queue's job is the work waiting on it. They are here
  // so the queue can still SEE them, which is the whole reason a withdrawal is a
  // state and not a delete.
  //
  // Fetched only while the tab is open, unlike the rescue queue: that one is
  // always fetched so a "needs attention" badge can appear unprompted, and this
  // one has nothing to prompt about.
  const {
    data: withdrawnData,
    isLoading: withdrawnLoading,
    error: withdrawnError,
  } = useAdminEntityRequests({
    state: 'withdrawn',
    entity_type: entityTypeFilter || undefined,
    enabled: itemTypeFilter === 'withdrawn',
  })

  const isLoading =
    editsLoading ||
    reportsLoading ||
    commentsLoading ||
    requestsLoading ||
    rescueLoading ||
    withdrawnLoading
  const error =
    editsError ||
    reportsError ||
    commentsError ||
    requestsError ||
    rescueError ||
    withdrawnError

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
    const rescueItems: ModerationItem[] = (rescueData?.requests || []).map(r => ({
      type: 'rescue' as const,
      data: r,
    }))
    const withdrawnItems: ModerationItem[] = (withdrawnData?.requests || []).map(
      r => ({ type: 'withdrawn' as const, data: r })
    )

    // 'needs_attention' and 'withdrawn' are SEPARATE review surfaces, each its
    // own decision_state: approved-but-unfulfilled rows and retracted ones,
    // never the pending queue. The other four filters (and 'all') show the
    // pending queue and exclude both.
    let merged: ModerationItem[]
    if (itemTypeFilter === 'needs_attention') {
      merged = rescueItems
    } else if (itemTypeFilter === 'withdrawn') {
      merged = withdrawnItems
    } else {
      merged = [...editItems, ...reportItems, ...commentItems, ...requestItems]
      if (itemTypeFilter === 'edits') {
        merged = merged.filter(i => i.type === 'edit')
      } else if (itemTypeFilter === 'reports') {
        merged = merged.filter(i => i.type === 'report')
      } else if (itemTypeFilter === 'comments') {
        merged = merged.filter(i => i.type === 'comment')
      } else if (itemTypeFilter === 'requests') {
        merged = merged.filter(i => i.type === 'request')
      }
    }

    // Sort oldest first (review fairness) on every surface that is a QUEUE.
    // The withdrawn tab is not one: nothing there is waiting to be worked, so
    // the useful order is the most recently withdrawn first, which is also the
    // order the fixed server-side window selects.
    if (itemTypeFilter === 'withdrawn') {
      return [...merged].sort(
        (a, b) =>
          new Date((b.data as AdminEntityRequest).decided_at ?? b.data.created_at).getTime() -
          new Date((a.data as AdminEntityRequest).decided_at ?? a.data.created_at).getTime()
      )
    }

    merged = [...merged].sort(
      (a, b) =>
        new Date(a.data.created_at).getTime() - new Date(b.data.created_at).getTime()
    )

    return merged
  }, [
    editsData,
    reportsData,
    commentsData,
    requestsData,
    rescueData,
    withdrawnData,
    itemTypeFilter,
  ])

  const totalEdits = editsData?.total || 0
  const totalReports = reportsData?.total || 0
  const totalComments = commentsData?.total || 0
  const totalRequests = requestsData?.total || 0
  const totalRescue = rescueData?.total || 0
  const totalWithdrawn = withdrawnData?.total || 0
  // 'All' is the pending queue total; rescues (approved-but-unfulfilled) are a
  // separate surface and are NOT folded in here.
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
          {/* PSY-1088: approved-but-unfulfilled rescue queue. Separate from the
              pending tabs; only shown when there's something to rescue (or it's
              the active tab) so a clear queue doesn't carry a dead tab. */}
          {(totalRescue > 0 || itemTypeFilter === 'needs_attention') && (
            <FilterButton
              active={itemTypeFilter === 'needs_attention'}
              onClick={() => setItemTypeFilter('needs_attention')}
              label="Needs attention"
              count={totalRescue}
            />
          )}
          {/* PSY-1992: withdrawn requests. Always present, unlike the rescue
              tab, because its count is only fetched while it is open, so
              hiding it on a zero count would hide it permanently. */}
          <FilterButton
            active={itemTypeFilter === 'withdrawn'}
            onClick={() => setItemTypeFilter('withdrawn')}
            label="Withdrawn"
            count={totalWithdrawn}
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
          {items.length} item{items.length !== 1 ? 's' : ''}{' '}
          {itemTypeFilter === 'needs_attention'
            ? 'needing attention'
            : itemTypeFilter === 'withdrawn'
              ? 'withdrawn'
              : 'pending review'}
        </span>
      </div>

      {/* Empty state */}
      {items.length === 0 && (
        <AdminEmptyState
          icon={Inbox}
          title="Queue Clear"
          message={EMPTY_QUEUE_MESSAGE[itemTypeFilter]}
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
            if (item.type === 'rescue') {
              return (
                <RescueCard
                  key={`rescue-${item.data.id}`}
                  request={item.data as AdminEntityRequest}
                  onActionSuccess={handleActionSuccess}
                />
              )
            }
            if (item.type === 'withdrawn') {
              return (
                <WithdrawnRequestCard
                  key={`withdrawn-${item.data.id}`}
                  request={item.data as AdminEntityRequest}
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
    case 'voided':
      // No notification is sent on a void (the submitter saw the request as
      // approved); don't claim one, unlike the reject copy.
      return `Voided — ${action.entityLabel} dismissed`
    default:
      return 'Rejected — submitter notified of reason'
  }
}

// ─── Empty-state copy ────────────────────────────────────────────────────────

/**
 * What an empty queue says, per filter. A Record rather than a ternary chain so
 * a new filter is a compile error here instead of silently inheriting the copy
 * for "everything", which describes the pending queue and nothing else.
 */
const EMPTY_QUEUE_MESSAGE: Record<ItemTypeFilter, string> = {
  all: 'No items need moderation. Pending entity edits, reports, comments, and creation requests will appear here when users submit them.',
  edits: 'No pending entity edits to review.',
  reports: 'No pending entity reports to review.',
  comments: 'No pending comments to review.',
  requests: 'No pending entity-creation requests to review.',
  needs_attention:
    'No approved-but-unfulfilled requests. Anything approved whose entity was never created would appear here to fulfill or void.',
  withdrawn:
    'No withdrawn requests. A request its requester retracted while it was still pending would appear here, for the record.',
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
