import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { LowQualityTagQueueItem } from '../types'

const mockUseLowQualityTagQueue = vi.fn()
const mockSnooze = vi.fn()
const mockMarkOfficial = vi.fn()
const mockDelete = vi.fn()
const mockBulkAction = vi.fn()

vi.mock('./useAdminTags', () => ({
  useLowQualityTagQueue: (...args: unknown[]) =>
    mockUseLowQualityTagQueue(...args),
  useSnoozeTag: () => ({
    mutate: mockSnooze,
    isPending: false,
    variables: undefined,
  }),
  useMarkTagOfficial: () => ({
    mutate: mockMarkOfficial,
    isPending: false,
    variables: undefined,
  }),
  useDeleteTag: () => ({
    mutate: mockDelete,
    isPending: false,
  }),
  useBulkLowQualityAction: () => ({
    mutate: mockBulkAction,
    isPending: false,
  }),
  // MergeTagDialog transitively needs these.
  useMergeTags: () => ({ mutate: vi.fn(), isPending: false }),
  useMergeTagsPreview: () => ({ data: null, isLoading: false, error: null }),
  useTagAliases: () => ({ data: { aliases: [] }, isLoading: false }),
}))

vi.mock('../hooks', () => ({
  useSearchTags: () => ({ data: { tags: [] }, isLoading: false }),
}))

import { LowQualityTagQueue } from './LowQualityTagQueue'

function makeItem(
  overrides: Partial<LowQualityTagQueueItem> = {}
): LowQualityTagQueueItem {
  return {
    id: 1,
    name: 'rock',
    slug: 'rock',
    category: 'genre',
    is_official: false,
    usage_count: 0,
    created_at: '2025-01-01T00:00:00Z',
    upvotes: 0,
    downvotes: 0,
    reasons: ['orphaned'],
    ...overrides,
  }
}

describe('LowQualityTagQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the empty state when no tags are flagged', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: { tags: [], total: 0 },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText(/nothing to review/i)).toBeInTheDocument()
  })

  it('renders each flagged tag with its reason pills', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({
            id: 1,
            name: 'orphan-tag',
            slug: 'orphan-tag',
            reasons: ['orphaned', 'short_name'],
          }),
          makeItem({
            id: 2,
            name: 'downvoted-tag',
            slug: 'downvoted-tag',
            usage_count: 3,
            upvotes: 1,
            downvotes: 4,
            reasons: ['downvoted'],
          }),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText('orphan-tag')).toBeInTheDocument()
    expect(screen.getByText('downvoted-tag')).toBeInTheDocument()

    const orphanReasons = screen.getByTestId('reasons-1')
    expect(orphanReasons).toHaveTextContent('Orphaned')
    expect(orphanReasons).toHaveTextContent('Short name')

    const downvotedReasons = screen.getByTestId('reasons-2')
    expect(downvotedReasons).toHaveTextContent('Downvoted')
  })

  it('fires the snooze mutation when Ignore is clicked', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 42, name: 'mystery' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    fireEvent.click(
      screen.getByRole('button', { name: /ignore mystery for 30 days/i })
    )
    expect(mockSnooze).toHaveBeenCalledWith(42)
  })

  it('fires the mark-official mutation when Official is clicked', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 7, name: 'goodbadtag' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    fireEvent.click(
      screen.getByRole('button', { name: /mark goodbadtag official/i })
    )
    expect(mockMarkOfficial).toHaveBeenCalledWith(7)
  })

  it('shows loading state while fetching', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    })

    const { container } = renderWithProviders(<LowQualityTagQueue />)

    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('surfaces errors', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
    })

    renderWithProviders(<LowQualityTagQueue />)

    expect(screen.getByText(/boom/)).toBeInTheDocument()
  })

  it('paginates with Previous/Next', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem()],
        total: 50,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)

    const prevBtn = screen.getByRole('button', { name: /^previous$/i })
    const nextBtn = screen.getByRole('button', { name: /^next$/i })
    expect(prevBtn).toBeDisabled() // offset=0
    expect(nextBtn).toBeEnabled() // 50 total, 20 per page

    fireEvent.click(nextBtn)
    // After click, the hook is called again with offset=20. We can verify
    // via the latest call args.
    const callArgs = mockUseLowQualityTagQueue.mock.calls.at(-1)?.[0]
    expect(callArgs).toMatchObject({ limit: 20, offset: 20 })
  })

  // ──────────────────────────────────────────────
  // PSY-487 — bulk select + filter chips + typed confirm
  // ──────────────────────────────────────────────

  it('toolbar is hidden until at least one row is selected', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 1, name: 'aaa' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    expect(screen.queryByTestId('bulk-action-toolbar')).toBeNull()

    fireEvent.click(screen.getByTestId('row-checkbox-1'))
    expect(screen.getByTestId('bulk-action-toolbar')).toBeInTheDocument()
    expect(screen.getByText(/^1 selected$/)).toBeInTheDocument()
  })

  it('select-all-visible toggles every checkbox on the page', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({ id: 1, name: 'aaa' }),
          makeItem({ id: 2, name: 'bbb' }),
          makeItem({ id: 3, name: 'ccc' }),
        ],
        total: 3,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('select-all-visible'))
    expect(screen.getByText(/^3 selected$/)).toBeInTheDocument()

    // Toggle off
    fireEvent.click(screen.getByTestId('select-all-visible'))
    expect(screen.queryByTestId('bulk-action-toolbar')).toBeNull()
  })

  it('bulk Ignore opens a confirm dialog and fires the snooze bulk action', async () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({ id: 1, name: 'aaa' }),
          makeItem({ id: 2, name: 'bbb' }),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('row-checkbox-1'))
    fireEvent.click(screen.getByTestId('row-checkbox-2'))
    fireEvent.click(screen.getByTestId('bulk-action-snooze'))

    // Confirm in the dialog
    const confirmBtn = await screen.findByTestId('bulk-snooze-confirm')
    fireEvent.click(confirmBtn)

    expect(mockBulkAction).toHaveBeenCalledTimes(1)
    expect(mockBulkAction).toHaveBeenCalledWith(
      { action: 'snooze', tagIds: [1, 2] },
      expect.any(Object)
    )
  })

  it('bulk Mark Official fires with the mark_official verb', async () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [makeItem({ id: 5, name: 'ee' })],
        total: 1,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('row-checkbox-5'))
    fireEvent.click(screen.getByTestId('bulk-action-mark-official'))

    const confirmBtn = await screen.findByTestId('bulk-mark-official-confirm')
    fireEvent.click(confirmBtn)

    expect(mockBulkAction).toHaveBeenCalledWith(
      { action: 'mark_official', tagIds: [5] },
      expect.any(Object)
    )
  })

  it('bulk Delete with small selection skips the typed-confirm gate', async () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({ id: 1 }),
          makeItem({ id: 2 }),
          makeItem({ id: 3 }),
        ],
        total: 3,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('row-checkbox-1'))
    fireEvent.click(screen.getByTestId('row-checkbox-2'))
    fireEvent.click(screen.getByTestId('row-checkbox-3'))
    fireEvent.click(screen.getByTestId('bulk-action-delete'))

    // Typed-confirm input should NOT render for selections ≤ 5
    expect(screen.queryByTestId('bulk-delete-confirm-input')).toBeNull()

    const confirm = await screen.findByTestId('bulk-delete-confirm')
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)

    expect(mockBulkAction).toHaveBeenCalledWith(
      { action: 'delete', tagIds: [1, 2, 3] },
      expect.any(Object)
    )
  })

  it('bulk Delete with > 5 selections requires typing the exact phrase', async () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({ id: 1 }),
          makeItem({ id: 2 }),
          makeItem({ id: 3 }),
          makeItem({ id: 4 }),
          makeItem({ id: 5 }),
          makeItem({ id: 6 }),
        ],
        total: 6,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('select-all-visible'))
    fireEvent.click(screen.getByTestId('bulk-action-delete'))

    const confirm = await screen.findByTestId('bulk-delete-confirm')
    expect(confirm).toBeDisabled() // typed-confirm input is empty

    const input = screen.getByTestId('bulk-delete-confirm-input')
    fireEvent.change(input, { target: { value: 'wrong phrase' } })
    expect(confirm).toBeDisabled()

    fireEvent.change(input, { target: { value: 'delete 6 tags' } })
    expect(confirm).toBeEnabled()

    fireEvent.click(confirm)
    expect(mockBulkAction).toHaveBeenCalledWith(
      { action: 'delete', tagIds: [1, 2, 3, 4, 5, 6] },
      expect.any(Object)
    )
  })

  it('signal-type filter chips narrow the visible list to one signal', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({ id: 1, name: 'orphan-1', reasons: ['orphaned'] }),
          makeItem({
            id: 2,
            name: 'aging-1',
            usage_count: 1,
            reasons: ['aging_unused'],
          }),
          makeItem({
            id: 3,
            name: 'downvoted-1',
            usage_count: 5,
            reasons: ['downvoted'],
          }),
        ],
        total: 3,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    // All three visible by default.
    expect(screen.getByText('orphan-1')).toBeInTheDocument()
    expect(screen.getByText('aging-1')).toBeInTheDocument()
    expect(screen.getByText('downvoted-1')).toBeInTheDocument()

    // Activate "Aging unused" chip — only aging-1 should remain visible.
    fireEvent.click(screen.getByTestId('signal-chip-aging_unused'))
    expect(screen.queryByText('orphan-1')).toBeNull()
    expect(screen.getByText('aging-1')).toBeInTheDocument()
    expect(screen.queryByText('downvoted-1')).toBeNull()

    // Add "Downvoted" — both aging-1 and downvoted-1 should now be visible.
    fireEvent.click(screen.getByTestId('signal-chip-downvoted'))
    expect(screen.queryByText('orphan-1')).toBeNull()
    expect(screen.getByText('aging-1')).toBeInTheDocument()
    expect(screen.getByText('downvoted-1')).toBeInTheDocument()

    // Click "All" to reset.
    fireEvent.click(screen.getByRole('button', { name: 'All' }))
    expect(screen.getByText('orphan-1')).toBeInTheDocument()
    expect(screen.getByText('aging-1')).toBeInTheDocument()
    expect(screen.getByText('downvoted-1')).toBeInTheDocument()
  })

  it('"Unusual length" chip matches both short_name and long_name signals', () => {
    mockUseLowQualityTagQueue.mockReturnValue({
      data: {
        tags: [
          makeItem({
            id: 1,
            name: 'ok',
            slug: 'ok',
            reasons: ['short_name'],
          }),
          makeItem({
            id: 2,
            name: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
            slug: 'long-tag-1',
            reasons: ['long_name'],
          }),
          makeItem({
            id: 3,
            name: 'normal',
            slug: 'normal',
            reasons: ['orphaned'],
          }),
        ],
        total: 3,
      },
      isLoading: false,
      error: null,
    })

    renderWithProviders(<LowQualityTagQueue />)
    fireEvent.click(screen.getByTestId('signal-chip-unusual_length'))

    // short_name and long_name both show; orphaned does not.
    expect(screen.getByText('ok')).toBeInTheDocument()
    expect(
      screen.getByText('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
    ).toBeInTheDocument()
    expect(screen.queryByText('normal')).toBeNull()
  })
})
