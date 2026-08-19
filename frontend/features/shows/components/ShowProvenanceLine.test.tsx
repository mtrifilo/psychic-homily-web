import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ShowResponse } from '../types'
import type { EntityAttribution } from '@/features/contributions'

const mockUseEntityAttribution = vi.fn<() => { data: EntityAttribution | null }>(
  () => ({ data: null })
)
vi.mock('@/features/contributions', () => ({
  useEntityAttribution: () => mockUseEntityAttribution(),
}))

vi.mock('./ReportShowButton', () => ({
  ReportShowButton: ({ variant }: { variant?: string }) => (
    <button data-testid="report-button" data-variant={variant}>
      [Report issue]
    </button>
  ),
}))

import { ShowProvenanceLine } from './ShowProvenanceLine'

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-08-13T01:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 1,
        slug: 'salt-shed',
        name: 'Salt Shed',
        city: 'Chicago',
        state: 'IL',
        verified: true,
      },
    ],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}

function renderLine(
  show: ShowResponse,
  { canEdit = false, onEdit = vi.fn() } = {}
) {
  render(
    <ShowProvenanceLine
      show={show}
      showTitle="Test Show"
      canEdit={canEdit}
      onEdit={onEdit}
    />
  )
  return { onEdit }
}

describe('ShowProvenanceLine', () => {
  beforeEach(() => {
    // Only Date is faked (shortDate compares against the current year);
    // real timers stay live so testing-library's event loop is untouched.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-08-18T12:00:00Z'))
    mockUseEntityAttribution.mockReturnValue({ data: null })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('credits the calendar the listing was scraped from', () => {
    renderLine(makeShow({ source: 'discovery', source_venue: 'salt-shed' }))
    expect(
      screen.getByText('Listing from Salt Shed calendar')
    ).toBeInTheDocument()
  })

  it('credits nobody for a user-submitted show', () => {
    renderLine(makeShow())
    expect(screen.queryByText(/Listing from/)).not.toBeInTheDocument()
  })

  it('dates the record absolutely, year only when it is not the current one', () => {
    renderLine(makeShow({ created_at: '2024-01-15T12:00:00Z' }))
    expect(screen.getByText(/added Jan 15, 2024/)).toBeInTheDocument()
  })

  it('drops the year for a current-year record', () => {
    renderLine(makeShow({ created_at: '2026-07-12T12:00:00Z' }))
    expect(screen.getByText(/added Jul 12(?!,)/)).toBeInTheDocument()
  })

  it('renders the last edit, its editor, and the edit count from the revisions read', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'mtrifilo',
        user_username: 'mtrifilo',
        created_at: '2026-07-31T12:00:00Z',
        total: 3,
      },
    })
    renderLine(makeShow())

    expect(screen.getByText(/updated Jul 31 by/)).toBeInTheDocument()
    expect(screen.getByText('mtrifilo')).toBeInTheDocument()
    expect(screen.getByText('3 edits')).toBeInTheDocument()
  })

  it('says edit, singular, for one revision', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'mtrifilo',
        user_username: null,
        created_at: '2026-07-31T12:00:00Z',
        total: 1,
      },
    })
    renderLine(makeShow())
    expect(screen.getByText('1 edit')).toBeInTheDocument()
  })

  it('renders no update fragments before the first revision', () => {
    renderLine(makeShow())
    expect(screen.queryByText(/updated/)).not.toBeInTheDocument()
    expect(screen.queryByText(/edits?$/)).not.toBeInTheDocument()
  })

  // Shows have no suggest pipeline: the affordance exists only for viewers
  // whose drawer actually opens, honestly labelled. The accessible name is
  // distinguished from ShowActions' plain "Edit" button so an admin's page
  // never announces two identical Edit controls.
  it('renders a working Edit bracket only for viewers who can edit', () => {
    const { onEdit } = renderLine(makeShow(), { canEdit: true })

    fireEvent.click(
      screen.getByRole('button', { name: 'Edit this show listing' })
    )
    expect(onEdit).toHaveBeenCalledTimes(1)
  })

  it('omits the Edit bracket for viewers with no edit path', () => {
    renderLine(makeShow(), { canEdit: false })
    expect(
      screen.queryByRole('button', { name: 'Edit this show listing' })
    ).not.toBeInTheDocument()
  })

  // The hook's response type is a hand-written mirror; if the count ever
  // goes missing the whole fragment must vanish — not render "undefined
  // edits", and not a dangling word "edits" either (asserted on the word,
  // because React renders {undefined} as nothing and a substring check on
  // "undefined" passes vacuously).
  it('omits the edit-count fragment entirely when the total is not a number', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'mtrifilo',
        user_username: null,
        created_at: '2026-07-31T12:00:00Z',
        total: undefined as unknown as number,
      },
    })
    renderLine(makeShow())
    expect(screen.getByText(/updated Jul 31/)).toBeInTheDocument()
    expect(
      screen.getByTestId('show-provenance-line').textContent
    ).not.toMatch(/\bedits?\b/)
  })

  // A reported ZERO renders — "0 edits" beside a revision we just named is
  // the loud contradiction that gets a backend regression reported, and the
  // hook's comment explicitly rejects quietly masking it.
  it('renders a reported zero loudly rather than hiding it', () => {
    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'mtrifilo',
        user_username: null,
        created_at: '2026-07-31T12:00:00Z',
        total: 0,
      },
    })
    renderLine(makeShow())
    expect(screen.getByText('0 edits')).toBeInTheDocument()
  })

  it('always offers Report issue, in the bracket register', () => {
    renderLine(makeShow())
    expect(screen.getByTestId('report-button')).toHaveAttribute(
      'data-variant',
      'bracket'
    )
  })

  // The separator logic is positional: when leading fragments are absent the
  // line must not open with a dangling middot.
  it('never leads with a separator', () => {
    renderLine(makeShow({ created_at: 'not-a-date' }))
    const text = screen.getByTestId('show-provenance-line').textContent ?? ''
    expect(text.trim().startsWith('·')).toBe(false)
  })

  // The revision fragments splice in when their async read resolves. The
  // list is keyed by stable fragment identity, so the report affordance
  // keeps its DOM node — and therefore its open-dialog state — across the
  // insertion. An index-keyed list remounts it and closes the dialog under
  // the user.
  it('keeps the report affordance mounted when the revision fragments arrive', () => {
    mockUseEntityAttribution.mockReturnValue({ data: null })
    const show = makeShow()
    const { rerender } = render(
      <ShowProvenanceLine
        show={show}
        showTitle="Test Show"
        canEdit={false}
        onEdit={vi.fn()}
      />
    )
    const before = screen.getByTestId('report-button')

    mockUseEntityAttribution.mockReturnValue({
      data: {
        user_name: 'mtrifilo',
        user_username: 'mtrifilo',
        created_at: '2026-07-31T12:00:00Z',
        total: 3,
      },
    })
    rerender(
      <ShowProvenanceLine
        show={show}
        showTitle="Test Show"
        canEdit={false}
        onEdit={vi.fn()}
      />
    )

    expect(screen.getByText('3 edits')).toBeInTheDocument()
    expect(screen.getByTestId('report-button')).toBe(before)
  })
})
