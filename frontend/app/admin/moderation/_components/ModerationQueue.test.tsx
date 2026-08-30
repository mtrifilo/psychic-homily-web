import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModerationQueue } from './ModerationQueue'
import { SET_TYPE_OPTIONS } from '@/features/shows/components/show-form-utils'
import type { PendingEditResponse } from '@/lib/hooks/admin/useAdminPendingEdits'
import type { EntityReportResponse } from '@/lib/hooks/admin/useAdminEntityReports'
import type { PendingComment } from '@/lib/hooks/admin/useAdminComments'
import type { AdminEntityRequest } from '@/lib/hooks/admin/useAdminEntityRequests'

// --- Mock data ---

const mockPendingEdit: PendingEditResponse = {
  id: 1,
  entity_type: 'artist',
  entity_id: 10,
  entity_name: 'Test Artist',
  submitted_by: 2,
  submitter_name: 'editor1',
  // PSY-619: covers the unlinked-byline path (account with no username).
  submitter_username: null,
  reviewer_username: null,
  field_changes: [{ field: 'name', old_value: 'Old', new_value: 'New' }],
  summary: 'Updated name',
  status: 'pending' as const,
  created_at: '2026-04-01T00:00:00Z',
  updated_at: '2026-04-01T00:00:00Z',
}

const mockEntityReport: EntityReportResponse = {
  id: 2,
  entity_type: 'venue',
  entity_id: 20,
  entity_name: 'Test Venue',
  reported_by: 3,
  reporter_name: 'reporter1',
  reporter_username: null,
  reviewer_username: null,
  report_type: 'wrong_address',
  details: 'Address is outdated',
  status: 'pending',
  created_at: '2026-04-02T00:00:00Z',
}

// The pending-comment queue returns a plain CommentResponse — the same shape
// the public comment endpoints return. It carries no `entity_name` and no
// `trust_tier`; the previous fixture declared both, which is why the dead
// entity link / trust-tier badge in the card went unnoticed (PSY-1600).
const mockPendingComment: PendingComment = {
  id: 3,
  entity_type: 'artist',
  entity_id: 10,
  kind: 'comment',
  user_id: 4,
  author_name: 'commenter1',
  author_username: null,
  body: 'This is a pending comment body',
  body_html: '<p>This is a pending comment body</p>',
  depth: 0,
  visibility: 'pending',
  reply_permission: 'anyone',
  ups: 0,
  downs: 0,
  score: 0,
  is_edited: false,
  edit_count: 0,
  reply_count: 0,
  created_at: '2026-04-03T00:00:00Z',
  updated_at: '2026-04-03T00:00:00Z',
}

const mockCommentReport: EntityReportResponse = {
  id: 4,
  entity_type: 'comment',
  entity_id: 50,
  entity_name: 'Comment #50',
  reported_by: 5,
  reporter_name: 'reporter2',
  reporter_username: null,
  reviewer_username: null,
  report_type: 'spam',
  details: 'This is spam content',
  status: 'pending',
  created_at: '2026-04-04T00:00:00Z',
}

// PSY-357: collection-typed report payload. Includes entity_slug because
// the moderation card uses it to deep-link to the public collection page
// and to call the admin hide endpoint.
const mockCollectionReport: EntityReportResponse = {
  id: 5,
  entity_type: 'collection',
  entity_id: 60,
  entity_name: 'Test Collection',
  entity_slug: 'test-collection',
  reported_by: 6,
  reporter_name: 'reporter3',
  reporter_username: null,
  reviewer_username: null,
  report_type: 'spam',
  details: 'This collection is spam',
  status: 'pending',
  created_at: '2026-04-05T00:00:00Z',
}

// PSY-661: release-typed report payload. Includes entity_slug so the
// generic EntityReportCard can deep-link to the public /releases/{slug} page.
const mockReleaseReport: EntityReportResponse = {
  id: 7,
  entity_type: 'release',
  entity_id: 70,
  entity_name: 'In Rainbows',
  entity_slug: 'in-rainbows',
  reported_by: 7,
  reporter_name: 'reporter4',
  reporter_username: null,
  reviewer_username: null,
  report_type: 'wrong_cover_art',
  details: 'Cover art shows the wrong album',
  status: 'pending',
  created_at: '2026-04-06T00:00:00Z',
}

// PSY-666: label-typed report payload. Includes entity_slug so the generic
// EntityReportCard can deep-link to the public /labels/{slug} page.
const mockLabelReport: EntityReportResponse = {
  id: 8,
  entity_type: 'label',
  entity_id: 80,
  entity_name: 'Run For Cover Records',
  entity_slug: 'run-for-cover-records',
  reported_by: 8,
  reporter_name: 'reporter5',
  reporter_username: null,
  reviewer_username: null,
  report_type: 'wrong_image',
  details: 'The label logo is wrong',
  status: 'pending',
  created_at: '2026-04-07T00:00:00Z',
}

// PSY-871: queued entity-creation request. Carries the resolved requester +
// the typed payload (rendered key:value) + optional AI source_detail.
const mockEntityRequest: AdminEntityRequest = {
  id: 9,
  entity_type: 'artist',
  payload: { name: 'Queued Band', city: 'Phoenix' },
  source_context: 'ai_extraction',
  source_detail: {
    url: 'https://example.com/article',
    excerpt: 'a great new band announced a tour',
  },
  requester_id: 9,
  requester_name: 'requester9',
  requester_username: null,
  decision_state: 'pending',
  created_at: '2026-04-08T00:00:00Z',
}

// --- Mocks ---

const mockUseAdminPendingEdits = vi.fn()
const mockUseApprovePendingEdit = vi.fn()
const mockUseRejectPendingEdit = vi.fn()
const mockUseAdminEntityReports = vi.fn()
const mockUseResolveEntityReport = vi.fn()
const mockUseDismissEntityReport = vi.fn()
const mockUseAdminHideCollection = vi.fn()
const mockUseAdminPendingComments = vi.fn()
const mockUseAdminApproveComment = vi.fn()
const mockUseAdminRejectComment = vi.fn()
const mockUseAdminHideComment = vi.fn()
const mockUseAdminEntityRequests = vi.fn()
const mockUseDecideEntityRequest = vi.fn()
const mockUseRescueEntityRequest = vi.fn()

const defaultMutationReturn = { mutate: vi.fn(), isPending: false, isError: false, error: null as Error | null }

vi.mock('@/lib/hooks/admin/useAdminPendingEdits', () => ({
  useAdminPendingEdits: (...args: unknown[]) => mockUseAdminPendingEdits(...args),
  useApprovePendingEdit: () => mockUseApprovePendingEdit(),
  useRejectPendingEdit: () => mockUseRejectPendingEdit(),
}))

vi.mock('@/lib/hooks/admin/useAdminEntityReports', () => ({
  useAdminEntityReports: (...args: unknown[]) => mockUseAdminEntityReports(...args),
  useResolveEntityReport: () => mockUseResolveEntityReport(),
  useDismissEntityReport: () => mockUseDismissEntityReport(),
  // PSY-357: hide-collection mutation, only invoked from CollectionReportCard.
  useAdminHideCollection: () => mockUseAdminHideCollection(),
}))

vi.mock('@/lib/hooks/admin/useAdminComments', () => ({
  useAdminPendingComments: (...args: unknown[]) => mockUseAdminPendingComments(...args),
  useAdminApproveComment: () => mockUseAdminApproveComment(),
  useAdminRejectComment: () => mockUseAdminRejectComment(),
  useAdminHideComment: () => mockUseAdminHideComment(),
  useAdminCommentEditHistory: () => ({ data: undefined as unknown, isLoading: false, error: null as Error | null }),
}))

vi.mock('@/lib/hooks/admin/useAdminEntityRequests', () => ({
  useAdminEntityRequests: (...args: unknown[]) => mockUseAdminEntityRequests(...args),
  useDecideEntityRequest: () => mockUseDecideEntityRequest(),
  useRescueEntityRequest: () => mockUseRescueEntityRequest(),
}))

// PSY-297: stub the edit-history dialog so the badge interaction test doesn't
// depend on Radix Dialog or the query client.
vi.mock('@/features/comments', () => ({
  CommentEditHistory: ({ open }: { open: boolean }) =>
    open ? <div data-testid="stub-edit-history-dialog" /> : null,
}))

describe('ModerationQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseApprovePendingEdit.mockReturnValue(defaultMutationReturn)
    mockUseRejectPendingEdit.mockReturnValue(defaultMutationReturn)
    mockUseResolveEntityReport.mockReturnValue(defaultMutationReturn)
    mockUseDismissEntityReport.mockReturnValue(defaultMutationReturn)
    mockUseAdminApproveComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminRejectComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminHideComment.mockReturnValue(defaultMutationReturn)
    mockUseAdminHideCollection.mockReturnValue(defaultMutationReturn)
    mockUseDecideEntityRequest.mockReturnValue(defaultMutationReturn)
    mockUseRescueEntityRequest.mockReturnValue(defaultMutationReturn)
  })

  function setDefaultMocks(overrides?: {
    edits?: unknown[]
    reports?: unknown[]
    comments?: unknown[]
    requests?: unknown[]
    // PSY-1088: approved-but-unfulfilled rescue rows (the second
    // useAdminEntityRequests call, with state=approved + unfulfilled=true).
    rescue?: unknown[]
  }) {
    mockUseAdminPendingEdits.mockReturnValue({
      data: { edits: overrides?.edits ?? [], total: overrides?.edits?.length ?? 0 },
      isLoading: false,
      error: null,
    })
    mockUseAdminEntityReports.mockReturnValue({
      data: { reports: overrides?.reports ?? [], total: overrides?.reports?.length ?? 0 },
      isLoading: false,
      error: null,
    })
    mockUseAdminPendingComments.mockReturnValue({
      data: { comments: overrides?.comments ?? [], total: overrides?.comments?.length ?? 0 },
      isLoading: false,
      error: null,
    })
    // The queue calls useAdminEntityRequests TWICE: the pending queue
    // (state='pending') and the rescue queue (state='approved' +
    // unfulfilled=true). Route by the filter arg so each gets its own data.
    mockUseAdminEntityRequests.mockImplementation((filters?: { unfulfilled?: boolean }) => {
      const rows = filters?.unfulfilled ? (overrides?.rescue ?? []) : (overrides?.requests ?? [])
      return {
        data: { requests: rows, total: rows.length },
        isLoading: false,
        error: null,
      }
    })
  }

  it('renders empty state when no items', () => {
    setDefaultMocks()

    render(<ModerationQueue />)

    expect(screen.getByText('Queue Clear')).toBeInTheDocument()
  })

  it('renders pending edit card', () => {
    setDefaultMocks({ edits: [mockPendingEdit] })

    render(<ModerationQueue />)

    expect(screen.getByText('Edit')).toBeInTheDocument()
    expect(screen.getByText('Artist')).toBeInTheDocument()
    expect(screen.getByText('Test Artist')).toBeInTheDocument()
  })

  // PSY-1600: `field_changes` is a nil-able Go slice — the backend leaves it
  // nil both when the column is NULL and when the stored JSON fails to
  // unmarshal, so the wire sends `null`. The old hand-written type declared it
  // non-null and the card read `.length` straight off it, throwing the whole
  // moderation queue away for one bad row.
  it('renders a pending edit whose field_changes came back null', () => {
    setDefaultMocks({ edits: [{ ...mockPendingEdit, field_changes: null }] })

    render(<ModerationQueue />)

    expect(screen.getByText('Test Artist')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /0 field changes/ })
    ).toBeInTheDocument()
  })

  it('renders entity report card', () => {
    setDefaultMocks({ reports: [mockEntityReport] })

    render(<ModerationQueue />)

    expect(screen.getByText('Report')).toBeInTheDocument()
    expect(screen.getByText('Venue')).toBeInTheDocument()
    expect(screen.getByText('Test Venue')).toBeInTheDocument()
  })

  it('renders pending comment card', () => {
    setDefaultMocks({ comments: [mockPendingComment] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('pending-comment-card')).toBeInTheDocument()
    expect(screen.getByText('Comment')).toBeInTheDocument()
    // PSY-613: byline is now rendered via the shared UserAttribution
    // primitive, which puts the name in its own span. Match the byline by
    // querying for the name text — the surrounding "by " stays a sibling
    // text node — rather than a single combined string.
    expect(screen.getByText('commenter1')).toBeInTheDocument()
    expect(screen.getByTestId('comment-body')).toBeInTheDocument()
  })

  // PSY-871: the 4th card type — queued entity-creation requests.
  it('renders entity request card with payload preview + Create action', () => {
    setDefaultMocks({ requests: [mockEntityRequest] })

    render(<ModerationQueue />)

    // Purple "Request" category badge + entity type + payload-derived label.
    expect(screen.getByText('Request')).toBeInTheDocument()
    expect(screen.getByText('Artist')).toBeInTheDocument()
    expect(screen.getByText('Queued Band')).toBeInTheDocument()
    // Requester attribution (unlinked — no username).
    expect(screen.getByText('requester9')).toBeInTheDocument()
    // Payload preview shows the non-header fields as key:value (name/title are
    // the header, so they're omitted from the preview).
    expect(screen.getByText('city:')).toBeInTheDocument()
    expect(screen.queryByText('name:')).not.toBeInTheDocument()
    // Action label is "Create" (not "Approve"); reject stays available.
    expect(screen.getByRole('button', { name: /create/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /reject/i })).toBeInTheDocument()
  })

  it('shows the Requests filter tab', () => {
    setDefaultMocks({ requests: [mockEntityRequest] })

    render(<ModerationQueue />)

    expect(screen.getByText('Requests')).toBeInTheDocument()
  })

  it('fires the decide mutation with approved when Create is clicked', () => {
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [mockEntityRequest] })

    render(<ModerationQueue />)

    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 9, decision: 'approved' }),
      expect.anything()
    )
  })

  it('rejects a request with the trimmed reason', () => {
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [mockEntityRequest] })

    render(<ModerationQueue />)

    fireEvent.click(screen.getByRole('button', { name: /^reject$/i }))
    fireEvent.change(screen.getByPlaceholderText(/rejection reason/i), {
      target: { value: '  not notable  ' },
    })
    fireEvent.click(screen.getByRole('button', { name: /confirm reject/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 9, decision: 'rejected', note: 'not notable' }),
      expect.anything()
    )
  })

  it('renders the source line, safe external link, and excerpt for AI requests', () => {
    setDefaultMocks({ requests: [mockEntityRequest] })

    render(<ModerationQueue />)

    expect(screen.getByText(/via AI extraction/i)).toBeInTheDocument()
    const sourceLink = screen.getByRole('link', { name: /source/i })
    expect(sourceLink).toHaveAttribute('href', 'https://example.com/article')
    expect(screen.getByText(/a great new band announced a tour/i)).toBeInTheDocument()
  })

  // PSY-1037: show requests are fulfillable — Create opens the associations
  // form (venue + artists) instead of approving immediately.
  it('opens the show associations form when Create is clicked on a show request', () => {
    const showRequest: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 11,
      entity_type: 'show',
      payload: { title: 'Big Fest', event_date: '2026-07-01', city: 'Phoenix', state: 'AZ' },
      source_detail: null,
    }
    setDefaultMocks({ requests: [showRequest] })

    render(<ModerationQueue />)

    // Header uses the payload title; the preview omits the header'd title.
    expect(screen.getByText('Big Fest')).toBeInTheDocument()
    expect(screen.queryByText('title:')).not.toBeInTheDocument()
    expect(screen.getByText('event_date:')).toBeInTheDocument()
    // Create enabled, no manual-create hint.
    const createButton = screen.getByRole('button', { name: /^create$/i })
    expect(createButton).not.toBeDisabled()
    expect(screen.queryByText(/must be created\s+manually for now/i)).not.toBeInTheDocument()

    // Clicking Create opens the form (no mutation yet) with city/state
    // prefilled from the payload.
    fireEvent.click(createButton)
    expect(screen.getByLabelText('Venue name')).toBeInTheDocument()
    expect(screen.getByLabelText('Venue city')).toHaveValue('Phoenix')
    expect(screen.getByLabelText('Venue state')).toHaveValue('AZ')
    // Submit disabled until venue name + ≥1 artist are filled; the row Create
    // button disables while the form is open (Cancel is the only way to close).
    expect(screen.getByRole('button', { name: /create show/i })).toBeDisabled()
    expect(createButton).toBeDisabled()
  })

  it('submits the show approval with the collected venue + artists', () => {
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    const showRequest: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 11,
      entity_type: 'show',
      payload: { title: 'Big Fest', event_date: '2026-07-01', city: 'Phoenix', state: 'AZ' },
      source_detail: null,
    }
    setDefaultMocks({ requests: [showRequest] })

    render(<ModerationQueue />)

    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    fireEvent.change(screen.getByLabelText('Venue name'), {
      target: { value: '  Valley Bar  ' },
    })
    fireEvent.change(screen.getByLabelText('Artist 1 name'), {
      target: { value: 'Boris' },
    })
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), {
      target: { value: 'Earth' },
    })
    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    // PSY-1856: with no role stated on either act, set_type is ABSENT from
    // both entries (toHaveBeenCalledWith compares the array exactly, so an
    // extra key fails here) and is_headliner is an explicit false — which is
    // what keeps the backend from reading position 0 as the headliner.
    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 11,
        decision: 'approved',
        show_venue: { name: 'Valley Bar', city: 'Phoenix', state: 'AZ' },
        show_artists: [
          { name: 'Boris', is_headliner: false },
          { name: 'Earth', is_headliner: false },
        ],
      }),
      expect.anything()
    )
  })

  // ─── Bill roles (PSY-1856) ───────────────────────────────────────────────
  //
  // The paired UI for PSY-1705's set_type on decide/fulfill. The three things
  // worth pinning: the vocabulary is offered, a stated role reaches the
  // mutation, and an UNSTATED role omits the key rather than sending "" (a
  // present-but-empty set_type is a 422 — only an absent key means unknown).

  const showRequestForRoles: AdminEntityRequest = {
    ...mockEntityRequest,
    id: 11,
    entity_type: 'show',
    payload: { title: 'Big Fest', event_date: '2026-07-01', city: 'Phoenix', state: 'AZ' },
    source_detail: null,
  }

  /** Open the show form on the queued request and fill the venue. */
  function openShowFormWithVenue() {
    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    fireEvent.change(screen.getByLabelText('Venue name'), { target: { value: 'Valley Bar' } })
  }

  async function chooseBillRole(
    user: ReturnType<typeof userEvent.setup>,
    artistNumber: number,
    optionLabel: string
  ) {
    await user.click(
      screen.getByRole('combobox', { name: `Artist ${artistNumber} bill role` })
    )
    await user.click(await screen.findByRole('option', { name: optionLabel }))
  }

  it('replaces the headliner checkbox with an unstated-by-default bill role', async () => {
    const user = userEvent.setup()
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    // The checkbox is gone: headliner is a role, not a separate flag.
    expect(screen.queryByLabelText(/is headliner/i)).not.toBeInTheDocument()

    const roleSelect = screen.getByRole('combobox', { name: 'Artist 1 bill role' })
    expect(roleSelect).toHaveTextContent('Role not stated')

    await user.click(roleSelect)
    // Exact list AND exact order. The backend orders its vocabulary so the API
    // docs, the 422 message and the form selector all read the same way, so an
    // extra option, a missing one, or a reshuffle is a regression. A
    // presence-only loop would pass through all three.
    expect(await screen.findByRole('option', { name: 'Headliner' })).toBeInTheDocument()
    expect(screen.getAllByRole('option').map(option => option.textContent)).toEqual([
      'Role not stated',
      'Headliner',
      'Direct support',
      'Opener',
      'Special guest',
      'DJ',
      'Performer (slot unknown)',
    ])
  })

  it('returns a row to unstated, dropping the set_type it had already stated', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    // The admin thinks better of it. The key must LEAVE the payload, not linger
    // as a stale value and not become '': this round trip is the one transition
    // where a previously stated role could survive in row state.
    await chooseBillRole(user, 1, 'Role not stated')

    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    const submitted = mutate.mock.calls[0][0] as { show_artists: Record<string, unknown>[] }
    expect('set_type' in submitted.show_artists[0]).toBe(false)
    expect(submitted.show_artists[0]).toEqual({ name: 'Boris', is_headliner: false })
  })

  it('keeps each row on its own role when an earlier row is removed', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    await chooseBillRole(user, 2, 'Opener')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 3 name'), { target: { value: 'DJ Sleep' } })
    await chooseBillRole(user, 3, 'DJ')

    // Pins the index arithmetic in the remove handler: every surviving row
    // keeps its OWN name paired with its OWN role, rather than inheriting the
    // removed row's.
    //
    // It does NOT pin React key stability, and must not be read as doing so:
    // name and role are both controlled from `artists` state, so this passes
    // under index keys, stable ids, or random keys alike. The rows are keyed by
    // index today; the sibling ShowForm mints a `_clientId` per row precisely
    // because that breaks once a row owns uncontrolled state. Add the first
    // such field here (an autocomplete, say) and this test will not save you.
    fireEvent.click(screen.getByRole('button', { name: 'Remove artist 2' }))
    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        show_artists: [
          { name: 'Boris', is_headliner: true, set_type: 'headliner' },
          { name: 'DJ Sleep', is_headliner: false, set_type: 'dj' },
        ],
      }),
      expect.anything()
    )
  })

  // One case per value, mirroring ShowForm.test.tsx, so a regression names the
  // role it broke. Driven off SET_TYPE_OPTIONS rather than a hand-written list:
  // a seventh role then arrives with coverage instead of without it. This also
  // covers 'performer', the one value whose payload shape differs from the
  // unstated sentinel's despite both storing the same row, and which a
  // "simplification" of the omit branch would silently fold away.
  for (const option of SET_TYPE_OPTIONS) {
    it(`sends set_type "${option.value}" when "${option.label}" is chosen`, async () => {
      const user = userEvent.setup()
      const mutate = vi.fn()
      mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
      setDefaultMocks({ requests: [showRequestForRoles] })

      render(<ModerationQueue />)
      openShowFormWithVenue()

      fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
      await chooseBillRole(user, 1, option.label)
      fireEvent.click(screen.getByRole('button', { name: /create show/i }))

      expect(mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          show_artists: [
            {
              name: 'Boris',
              is_headliner: option.value === 'headliner',
              set_type: option.value,
            },
          ],
        }),
        expect.anything()
      )
    })
  }

  it('sends a whole stated bill in order, deriving is_headliner from each role', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    await chooseBillRole(user, 2, 'Direct support')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 3 name'), { target: { value: 'DJ Sleep' } })
    await chooseBillRole(user, 3, 'DJ')

    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        show_artists: [
          { name: 'Boris', is_headliner: true, set_type: 'headliner' },
          { name: 'Earth', is_headliner: false, set_type: 'direct_support' },
          { name: 'DJ Sleep', is_headliner: false, set_type: 'dj' },
        ],
      }),
      expect.anything()
    )
  })

  // Pinning a DELIBERATE consequence, not asserting a desirable one. The
  // sibling ShowForm's removeArtistAtIndex promotes the first survivor to
  // headliner on this transition; this form does not, because promoting would
  // infer a role from row order, which is the inference the vocabulary exists
  // to remove. The result is a bill that states a role and has no headliner,
  // which the backend classifies as having no headline slot at all. That
  // consequence was escalated and DECIDED: warn, allow, and reconcile the
  // display under PSY-1943. So the transition below is exactly what raises the
  // partial-bill warning, and this test pins that it still submits the bill the
  // admin described rather than promoting a survivor behind their back.
  it('leaves the bill headliner-less when the headliner row is removed', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    await chooseBillRole(user, 2, 'Direct support')

    fireEvent.click(screen.getByRole('button', { name: 'Remove artist 1' }))
    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        show_artists: [{ name: 'Earth', is_headliner: false, set_type: 'direct_support' }],
      }),
      expect.anything()
    )
  })

  it('omits set_type for an unstated act on a bill that states other roles', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })

    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    const submitted = mutate.mock.calls[0][0] as { show_artists: Record<string, unknown>[] }
    // The key is ABSENT, not "" and not null: a present set_type outside the
    // vocabulary is rejected, so "unstated" can only be expressed by omission.
    expect('set_type' in submitted.show_artists[1]).toBe(false)
    expect(submitted.show_artists[1]).toEqual({ name: 'Earth', is_headliner: false })
  })

  // A nameless row is dropped from the bill, so a role stated on one would be
  // discarded silently. Blocked at submit instead: the discarded thing is the
  // primary field an admin reaches for, and losing it can quietly cost the bill
  // the headliner somebody explicitly designated.
  it('blocks submit while a row states a role but has no name', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Headliner')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    await chooseBillRole(user, 2, 'Opener')

    const submit = screen.getByRole('button', { name: /create show/i })
    expect(submit).toBeDisabled()
    expect(screen.getByText(/Name the act you gave a role to/i)).toBeInTheDocument()
    fireEvent.click(submit)
    expect(mutate).not.toHaveBeenCalled()

    // Clearing the role releases the block, and the empty row stays out of the bill.
    await chooseBillRole(user, 2, 'Role not stated')
    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        show_artists: [{ name: 'Boris', is_headliner: true, set_type: 'headliner' }],
      }),
      expect.anything()
    )
  })

  // The invariant the "Role not stated" / "Performer (slot unknown)" equivalence
  // rests on. Drop the always-send and those two menu options start producing
  // different bills server-side, because buildShowAssociations reads a present
  // set_type as curation while suppressPositionInference only skips rows that
  // state a field. Enforced here rather than merely described in a comment.
  it('always sends is_headliner, even for an entirely unstated bill', async () => {
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    fireEvent.click(screen.getByRole('button', { name: /create show/i }))

    const submitted = mutate.mock.calls[0][0] as { show_artists: Record<string, unknown>[] }
    for (const artist of submitted.show_artists) {
      expect('is_headliner' in artist).toBe(true)
      expect('set_type' in artist).toBe(false)
    }
  })

  // ─── Partial-bill warning (PSY-1856) ─────────────────────────────────────
  //
  // A bill that curates a slot but names no headliner has NO headline slot in
  // charts, while the show page still renders the first act as the headliner.
  // The recorded decision is WARN, ALLOW: the form surfaces the state, does not
  // gate on it, and the display reconciliation is PSY-1943's.
  //
  // The pair worth holding together is "warns on the partial shape" and "stays
  // silent on an all-'performer' bill": 'performer' reads as a stated role in
  // this form and as "slot unknown" in headlineSlotSQL, so the warning has to
  // ask the backend's question, not the form's.

  const partialBillWarning = /No headliner stated/i

  it('warns, without blocking, when the bill states a role but names no headliner', async () => {
    const user = userEvent.setup()
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Opener')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })

    expect(screen.getByText(partialBillWarning)).toBeInTheDocument()

    // The whole point of the decision: this is a caution, not a gate. The bill
    // an admin describes honestly ("somebody opened, nobody topped it") still
    // submits, and submits unchanged.
    const submit = screen.getByRole('button', { name: /create show/i })
    expect(submit).toBeEnabled()
    fireEvent.click(submit)

    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        show_artists: [
          { name: 'Boris', is_headliner: false, set_type: 'opener' },
          { name: 'Earth', is_headliner: false },
        ],
      }),
      expect.anything()
    )
    // Still on screen after the submit: nothing about it is a submit-time gate.
    expect(screen.getByText(partialBillWarning)).toBeInTheDocument()
  })

  it('warns on a single-act bill whose one act is stated as something other than the headliner', async () => {
    const user = userEvent.setup()
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Direct support')

    // No second row to "promote", so there is genuinely no headline slot.
    expect(screen.getByText(partialBillWarning)).toBeInTheDocument()
  })

  it('stays silent while every act is unstated', () => {
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })

    // Nobody curated this bill, so headlineSlotSQL falls back to position 0 and
    // the top act DOES hold the headline slot. Warning here would be false.
    expect(screen.queryByText(partialBillWarning)).not.toBeInTheDocument()
  })

  it('stays silent for a bill of explicit "Performer (slot unknown)" rows', async () => {
    const user = userEvent.setup()
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Performer (slot unknown)')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    await chooseBillRole(user, 2, 'Performer (slot unknown)')

    // 'performer' is one of the two spellings of "slot unknown", so this bill is
    // uncurated server-side and keeps its position-0 headline slot. A warning
    // written as `set_type !== unstated` would fire here, wrongly.
    expect(screen.queryByText(partialBillWarning)).not.toBeInTheDocument()
  })

  it('clears the warning once an act is stated headliner', async () => {
    const user = userEvent.setup()
    setDefaultMocks({ requests: [showRequestForRoles] })

    render(<ModerationQueue />)
    openShowFormWithVenue()

    fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
    await chooseBillRole(user, 1, 'Opener')
    fireEvent.click(screen.getByRole('button', { name: /add artist/i }))
    fireEvent.change(screen.getByLabelText('Artist 2 name'), { target: { value: 'Earth' } })
    expect(screen.getByText(partialBillWarning)).toBeInTheDocument()

    // Naming a headliner anywhere on the bill resolves it: the rule is about
    // the bill, not about row 1.
    await chooseBillRole(user, 2, 'Headliner')
    expect(screen.queryByText(partialBillWarning)).not.toBeInTheDocument()
  })

  it('cancel closes the show form without mutating', () => {
    const mutate = vi.fn()
    mockUseDecideEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
    const showRequest: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 11,
      entity_type: 'show',
      payload: { title: 'Big Fest', event_date: '2026-07-01' },
      source_detail: null,
    }
    setDefaultMocks({ requests: [showRequest] })

    render(<ModerationQueue />)

    fireEvent.click(screen.getByRole('button', { name: /^create$/i }))
    expect(screen.getByLabelText('Venue name')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(screen.queryByLabelText('Venue name')).not.toBeInTheDocument()
    expect(mutate).not.toHaveBeenCalled()
  })

  // PSY-998: festival requests are now fulfillable on approve (series_slug is
  // derived backend-side), so the queue enables Create for them.
  it('enables Create for festival requests', () => {
    const festivalRequest: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 12,
      entity_type: 'festival',
      payload: { name: 'Desert Daze', start_date: '2026-09-01', end_date: '2026-09-03' },
      source_detail: null,
    }
    setDefaultMocks({ requests: [festivalRequest] })

    render(<ModerationQueue />)

    expect(screen.getByText('Desert Daze')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create/i })).not.toBeDisabled()
    expect(screen.queryByText(/must be created\s+manually for now/i)).not.toBeInTheDocument()
  })

  it('renders comment report card for comment-type reports', () => {
    setDefaultMocks({ reports: [mockCommentReport] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('comment-report-card')).toBeInTheDocument()
    expect(screen.getByText('Spam')).toBeInTheDocument()
    expect(screen.getByText('Hide Comment')).toBeInTheDocument()
    expect(screen.getByText('Dismiss Report')).toBeInTheDocument()
  })

  // PSY-357: collection reports get a dedicated card with a "Hide from
  // Public Browse" action that flips is_public=false. The slug is required
  // to render the link and to enable the Hide button.
  it('renders collection report card for collection-type reports', () => {
    setDefaultMocks({ reports: [mockCollectionReport] })

    render(<ModerationQueue />)

    expect(screen.getByTestId('collection-report-card')).toBeInTheDocument()
    expect(screen.getByText('Test Collection')).toBeInTheDocument()
    expect(screen.getByText('Hide from Public Browse')).toBeInTheDocument()
    expect(screen.getByText('Dismiss Report')).toBeInTheDocument()
  })

  it('disables Hide on collection report when slug is missing (deleted)', () => {
    setDefaultMocks({
      reports: [{ ...mockCollectionReport, entity_slug: undefined }],
    })

    render(<ModerationQueue />)

    const hideButton = screen.getByText('Hide from Public Browse').closest('button')
    expect(hideButton).toBeDisabled()
    // Dismiss is still available so admins can clear stale reports.
    const dismissButton = screen.getByText('Dismiss Report').closest('button')
    expect(dismissButton).not.toBeDisabled()
  })

  // PSY-661: release reports flow through the generic EntityReportCard (no
  // bespoke moderation action). The card must show the entity-type badge,
  // the release-tailored report-type label, and a slug-based deep-link.
  it('renders release report via the generic entity report card', () => {
    setDefaultMocks({ reports: [mockReleaseReport] })

    render(<ModerationQueue />)

    expect(screen.getByText('Release')).toBeInTheDocument()
    expect(screen.getByText('Wrong Cover Art')).toBeInTheDocument()
    const link = screen.getByText('In Rainbows').closest('a')
    expect(link).toHaveAttribute('href', '/releases/in-rainbows')
  })

  // PSY-666: label reports flow through the generic EntityReportCard (no
  // bespoke moderation action), the same path as releases. The card must show
  // the entity-type badge, the label-tailored report-type label, and a
  // slug-based deep-link.
  it('renders label report via the generic entity report card', () => {
    setDefaultMocks({ reports: [mockLabelReport] })

    render(<ModerationQueue />)

    expect(screen.getByText('Label')).toBeInTheDocument()
    expect(screen.getByText('Wrong Image')).toBeInTheDocument()
    const link = screen.getByText('Run For Cover Records').closest('a')
    expect(link).toHaveAttribute('href', '/labels/run-for-cover-records')
  })

  it('shows correct counts in filter buttons', () => {
    setDefaultMocks({
      edits: [mockPendingEdit],
      reports: [mockEntityReport, mockCommentReport],
      comments: [mockPendingComment],
    })

    render(<ModerationQueue />)

    // Total: 1 edit + 2 reports + 1 comment = 4
    expect(screen.getByText('4')).toBeInTheDocument() // All count
    expect(screen.getByText('2')).toBeInTheDocument() // Reports count
    // Edits (1) and Comments (1) have the same count, so use getAllByText
    const onesElems = screen.getAllByText('1')
    expect(onesElems.length).toBeGreaterThanOrEqual(2) // Edits + Comments
  })

  it('renders approve and reject buttons on pending comment card', () => {
    setDefaultMocks({ comments: [mockPendingComment] })

    render(<ModerationQueue />)

    const card = screen.getByTestId('pending-comment-card')
    expect(within(card).getByText('Approve')).toBeInTheDocument()
    expect(within(card).getByText('Reject')).toBeInTheDocument()
  })

  it('displays all item types in unified view', () => {
    setDefaultMocks({
      edits: [mockPendingEdit],
      reports: [mockEntityReport],
      comments: [mockPendingComment],
    })

    render(<ModerationQueue />)

    // Should show items from all three types
    expect(screen.getByText('Edit')).toBeInTheDocument()
    expect(screen.getByText('Report')).toBeInTheDocument()
    expect(screen.getByTestId('pending-comment-card')).toBeInTheDocument()
    expect(screen.getByText('3 items pending review')).toBeInTheDocument()
  })

  // ─── PSY-297: edit history badge on pending comment cards ─────────────────

  it('does NOT render the edit-count badge when the pending comment has no edits', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 0 }],
    })

    render(<ModerationQueue />)

    expect(
      screen.queryByTestId('pending-comment-edit-badge')
    ).not.toBeInTheDocument()
  })

  it('renders the edit-count badge with a pluralized label when the pending comment was edited', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 3 }],
    })

    render(<ModerationQueue />)

    const badge = screen.getByTestId('pending-comment-edit-badge')
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent('3 edits')
  })

  it('uses the singular form when the pending comment was edited exactly once', () => {
    setDefaultMocks({
      comments: [{ ...mockPendingComment, edit_count: 1 }],
    })

    render(<ModerationQueue />)

    expect(screen.getByTestId('pending-comment-edit-badge')).toHaveTextContent(
      '1 edit'
    )
  })

  // ─── PSY-603 / PSY-622: page-level success banner on Approve / Reject ────
  //
  // The banner state lives on ModerationQueue (not the card) because the card
  // unmounts on success when the pending-edits query invalidates. These tests
  // drive the success path by overriding the approve/reject mutation mocks to
  // immediately invoke the per-call onSuccess option.
  //
  // Banner DOM is the shared `EntitySaveSuccessBanner` (PSY-562 / PSY-622); we
  // query it via `role="status"` rather than a bespoke testid because the
  // primitive is intentionally the same on entity-detail pages and moderation.

  describe('Approve / Reject success banner (PSY-603 / PSY-622)', () => {
    function captureMutationOnSuccess() {
      // Approve takes (editId, options); reject takes (variables, options).
      // Both pass options as the SECOND argument, so the same shape works.
      const approveMutate = vi.fn(
        (_args: unknown, opts?: { onSuccess?: () => void }) => {
          opts?.onSuccess?.()
        }
      )
      const rejectMutate = vi.fn(
        (_args: unknown, opts?: { onSuccess?: () => void }) => {
          opts?.onSuccess?.()
        }
      )
      mockUseApprovePendingEdit.mockReturnValue({
        ...defaultMutationReturn,
        mutate: approveMutate,
      })
      mockUseRejectPendingEdit.mockReturnValue({
        ...defaultMutationReturn,
        mutate: rejectMutate,
      })
      return { approveMutate, rejectMutate }
    }

    it('does NOT render the banner before any action is taken', () => {
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)

      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })

    it('renders the success banner with entity name after Approve succeeds', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Approve'))

      const banner = screen.getByRole('status')
      expect(banner).toHaveTextContent('Approved')
      expect(banner).toHaveTextContent('Test Artist')
    })

    it('renders the success banner with submitter-notified copy after Reject succeeds', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      // Open the rejection-reason input, fill it, confirm.
      fireEvent.click(screen.getByText('Reject'))
      const textarea = screen.getByPlaceholderText(/Rejection reason/i)
      fireEvent.change(textarea, { target: { value: 'Inaccurate change' } })
      fireEvent.click(screen.getByText('Confirm Reject'))

      const banner = screen.getByRole('status')
      expect(banner).toHaveTextContent('Rejected')
      expect(banner).toHaveTextContent(/submitter notified/i)
    })

    it('auto-dismisses the banner after the timeout elapses', () => {
      vi.useFakeTimers()
      try {
        captureMutationOnSuccess()
        setDefaultMocks({ edits: [mockPendingEdit] })

        render(<ModerationQueue />)
        fireEvent.click(screen.getByText('Approve'))
        expect(screen.getByRole('status')).toBeInTheDocument()

        // Advance just past the 5s timeout.
        act(() => {
          vi.advanceTimersByTime(5001)
        })

        expect(screen.queryByRole('status')).not.toBeInTheDocument()
      } finally {
        vi.useRealTimers()
      }
    })

    it('clears the banner immediately when the admin switches filter tabs', () => {
      captureMutationOnSuccess()
      setDefaultMocks({ edits: [mockPendingEdit] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Approve'))
      expect(screen.getByRole('status')).toBeInTheDocument()

      // Click the "Reports" filter tab.
      fireEvent.click(screen.getByText('Reports'))

      expect(screen.queryByRole('status')).not.toBeInTheDocument()
    })
  })

  // ── PSY-1088: approved-but-unfulfilled rescue queue ──────────────────────
  describe('rescue queue (needs attention)', () => {
    const orphanArtist: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 50,
      entity_type: 'artist',
      payload: { name: 'Orphan Band', city: 'Phoenix' },
      source_detail: null,
      decision_state: 'approved',
      // Unfulfilled: the backend omits created_entity_id entirely (pointer +
      // omitempty), it never sends an explicit null.
      created_entity_id: undefined,
    }
    const orphanShow: AdminEntityRequest = {
      ...mockEntityRequest,
      id: 51,
      entity_type: 'show',
      payload: { title: 'Deferred Show', event_date: '2026-08-01', city: 'Phoenix', state: 'AZ' },
      source_detail: null,
      decision_state: 'approved',
      // Unfulfilled: the backend omits created_entity_id entirely (pointer +
      // omitempty), it never sends an explicit null.
      created_entity_id: undefined,
    }

    it('hides the Needs attention tab when there are no orphans', () => {
      setDefaultMocks({ requests: [mockEntityRequest] })
      render(<ModerationQueue />)
      expect(screen.queryByText('Needs attention')).not.toBeInTheDocument()
    })

    it('shows the Needs attention tab when an orphan exists', () => {
      setDefaultMocks({ rescue: [orphanArtist] })
      render(<ModerationQueue />)
      expect(screen.getByText('Needs attention')).toBeInTheDocument()
    })

    it('orphans do NOT appear in the pending queue', () => {
      // Rescue rows go to the rescue query only; the default "All" pending view
      // must not surface them.
      setDefaultMocks({ rescue: [orphanArtist] })
      render(<ModerationQueue />)
      expect(screen.queryByText('Orphan Band')).not.toBeInTheDocument()
      expect(screen.getByText('Queue Clear')).toBeInTheDocument()
    })

    it('renders the rescue card with Fulfill + Void actions', () => {
      setDefaultMocks({ rescue: [orphanArtist] })
      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))

      expect(screen.getByText('Orphan Band')).toBeInTheDocument()
      expect(screen.getByText(/Approved but never created/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /fulfill/i })).toBeInTheDocument()
      // Secondary action is "Void" (not "Reject") — it dismisses an approved
      // orphan, a distinct action with no submitter notification.
      expect(screen.getByRole('button', { name: /^void$/i })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /^reject$/i })).not.toBeInTheDocument()
    })

    it('fires the rescue mutation with action=fulfill for a non-show orphan', () => {
      const mutate = vi.fn()
      mockUseRescueEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
      setDefaultMocks({ rescue: [orphanArtist] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))
      fireEvent.click(screen.getByRole('button', { name: /fulfill/i }))

      expect(mutate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 50, action: 'fulfill' }),
        expect.anything()
      )
    })

    it('voids an orphan with the trimmed reason', () => {
      const mutate = vi.fn()
      mockUseRescueEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
      setDefaultMocks({ rescue: [orphanArtist] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))
      fireEvent.click(screen.getByRole('button', { name: /^void$/i }))
      fireEvent.change(screen.getByPlaceholderText(/reason for voiding/i), {
        target: { value: '  bad auto-approve  ' },
      })
      fireEvent.click(screen.getByRole('button', { name: /confirm void/i }))

      expect(mutate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 50, action: 'void', note: 'bad auto-approve' }),
        expect.anything()
      )
    })

    it('shows a void-specific success banner (no "submitter notified")', () => {
      mockUseRescueEntityRequest.mockReturnValue({
        ...defaultMutationReturn,
        mutate: (_args: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
      })
      setDefaultMocks({ rescue: [orphanArtist] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))
      fireEvent.click(screen.getByRole('button', { name: /^void$/i }))
      fireEvent.change(screen.getByPlaceholderText(/reason for voiding/i), {
        target: { value: 'bad auto-approve' },
      })
      fireEvent.click(screen.getByRole('button', { name: /confirm void/i }))

      const banner = screen.getByRole('status')
      expect(banner).toHaveTextContent(/voided/i)
      expect(banner).not.toHaveTextContent(/notified/i)
    })

    it('opens the show form on Fulfill and submits the collected associations', () => {
      const mutate = vi.fn()
      mockUseRescueEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
      setDefaultMocks({ rescue: [orphanShow] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))

      // Fulfill opens the associations form (no mutation yet), prefilled.
      fireEvent.click(screen.getByRole('button', { name: /^fulfill$/i }))
      expect(screen.getByLabelText('Venue name')).toBeInTheDocument()
      expect(screen.getByLabelText('Venue city')).toHaveValue('Phoenix')
      expect(mutate).not.toHaveBeenCalled()

      fireEvent.change(screen.getByLabelText('Venue name'), { target: { value: 'Valley Bar' } })
      fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
      fireEvent.click(screen.getByRole('button', { name: /create show/i }))

      expect(mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 51,
          action: 'fulfill',
          show_venue: { name: 'Valley Bar', city: 'Phoenix', state: 'AZ' },
          show_artists: [{ name: 'Boris', is_headliner: false }],
        }),
        expect.anything()
      )
    })

    // PSY-1856: the rescue path shares ShowCreateForm with the approve path,
    // so it has to carry set_type too — the two endpoints accept the same
    // field and a rescue is where a stale orphan most often gets its bill
    // stated for the first time.
    it('carries the stated bill roles through the rescue fulfill', async () => {
      const user = userEvent.setup()
      const mutate = vi.fn()
      mockUseRescueEntityRequest.mockReturnValue({ ...defaultMutationReturn, mutate })
      setDefaultMocks({ rescue: [orphanShow] })

      render(<ModerationQueue />)
      fireEvent.click(screen.getByText('Needs attention'))
      fireEvent.click(screen.getByRole('button', { name: /^fulfill$/i }))

      fireEvent.change(screen.getByLabelText('Venue name'), { target: { value: 'Valley Bar' } })
      fireEvent.change(screen.getByLabelText('Artist 1 name'), { target: { value: 'Boris' } })
      await user.click(screen.getByRole('combobox', { name: 'Artist 1 bill role' }))
      await user.click(await screen.findByRole('option', { name: 'Special guest' }))
      fireEvent.click(screen.getByRole('button', { name: /create show/i }))

      expect(mutate).toHaveBeenCalledWith(
        expect.objectContaining({
          action: 'fulfill',
          show_artists: [
            { name: 'Boris', is_headliner: false, set_type: 'special_guest' },
          ],
        }),
        expect.anything()
      )
    })
  })
})
