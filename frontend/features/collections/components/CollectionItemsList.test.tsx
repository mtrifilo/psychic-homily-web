import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { CollectionItemsList } from './CollectionItemsList'
import type { StagedCollectionItem } from './AddItemsPicker'

// PSY-957: integration coverage for AddItemsPanel's sticky-vs-auto-dismiss
// feedback — the headline behavior the banner-timer consolidation had to
// preserve/fix. The shared primitive is unit-tested in
// lib/hooks/common/useAutoDismissBanner.test.ts; THIS file pins that
// AddItemsPanel actually routes a thrown (network/5xx) error to the sticky
// path and a resolved response to the auto-dismiss path. Without it, a future
// edit that swaps showStickyFeedback↔showFeedback ships green (both render the
// same red banner; only dismiss timing differs).

const mockBulkAddMutateAsync = vi.fn()
let mockBulkAddIsPending = false

vi.mock('../hooks', () => ({
  useReorderCollectionItems: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useRemoveCollectionItem: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useUpdateCollectionItem: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useBulkAddCollectionItems: () => ({
    mutateAsync: mockBulkAddMutateAsync,
    isPending: mockBulkAddIsPending,
    isError: false,
    error: null,
  }),
}))

// Stub the picker: real entity-search behavior is covered in
// AddItemsPicker.test.tsx. Here we only need a way to stage one item so the
// submit button enables and handleSubmit runs.
const STAGED_ITEM: StagedCollectionItem = {
  entityType: 'artist',
  entityId: 1,
  name: 'Test Artist',
  subtitle: null,
}
vi.mock('./AddItemsPicker', () => ({
  AddItemsPicker: ({
    onStagedItemsChange,
  }: {
    onStagedItemsChange: (items: StagedCollectionItem[]) => void
  }) => (
    <button
      type="button"
      data-testid="stub-stage-item"
      onClick={() => onStagedItemsChange([STAGED_ITEM])}
    >
      stage one item
    </button>
  ),
}))

// DensityToggle pulls in shared-bundle deps we don't exercise here.
vi.mock('@/components/shared', () => ({
  DensityToggle: () => <div data-testid="density-toggle-stub" />,
}))

function renderEmptyCreatorList() {
  // items=[] ⇒ AddItemsPanel auto-opens (isAddItemsOpen defaults to true);
  // isCreator ⇒ the panel is gated visible; unranked ⇒ no dnd-kit reorder mount
  // (canReorder is isCreator && isRanked).
  return render(
    <CollectionItemsList
      items={[]}
      slug="test-collection"
      isCreator
      displayMode="unranked"
    />
  )
}

async function stageAndSubmit() {
  fireEvent.click(screen.getByTestId('stub-stage-item'))
  // handleSubmit awaits mutateAsync; flush its microtasks inside act so the
  // post-await setState (showFeedback / showStickyFeedback) is applied.
  await act(async () => {
    fireEvent.click(screen.getByTestId('add-items-picker-submit'))
  })
}

describe('AddItemsPanel feedback (PSY-957 sticky-vs-auto-dismiss)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockBulkAddMutateAsync.mockReset()
    mockBulkAddIsPending = false
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('auto-dismisses the success banner ~4s after a fully-successful add', async () => {
    mockBulkAddMutateAsync.mockResolvedValue({ added: [{ id: 1 }], errors: [] })
    renderEmptyCreatorList()

    await stageAndSubmit()

    expect(screen.getByTestId('add-item-success')).toBeInTheDocument()

    // Still up just before the window closes…
    act(() => {
      vi.advanceTimersByTime(3999)
    })
    expect(screen.queryByTestId('add-item-success')).toBeInTheDocument()

    // …gone at 4s.
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(screen.queryByTestId('add-item-success')).not.toBeInTheDocument()
  })

  it('auto-dismisses the resolved-rejection error banner ~4s (all rows rejected)', async () => {
    // A resolved response where every row failed is a per-row rejection, not a
    // thrown error — it stays on the auto-dismiss path (the user can retry).
    mockBulkAddMutateAsync.mockResolvedValue({
      added: [],
      errors: [{ reason: 'dupe' }],
    })
    renderEmptyCreatorList()

    await stageAndSubmit()

    expect(screen.getByTestId('add-item-error')).toBeInTheDocument()

    act(() => {
      vi.advanceTimersByTime(4000)
    })
    expect(screen.queryByTestId('add-item-error')).not.toBeInTheDocument()
  })

  it('keeps the thrown-error banner STICKY past the auto-dismiss window', async () => {
    // A rejected promise (network / 5xx) is a hard failure the user must read;
    // it must NOT auto-dismiss. This is the regression the swap-back would break.
    mockBulkAddMutateAsync.mockRejectedValue(new Error('network blew up'))
    renderEmptyCreatorList()

    await stageAndSubmit()

    expect(screen.getByTestId('add-item-error')).toHaveTextContent(
      'network blew up'
    )

    // Well past the 4s auto-dismiss window — still on screen.
    act(() => {
      vi.advanceTimersByTime(20_000)
    })
    expect(screen.getByTestId('add-item-error')).toHaveTextContent(
      'network blew up'
    )
  })
})
