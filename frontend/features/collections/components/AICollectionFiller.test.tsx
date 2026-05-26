import { describe, it, expect, vi, beforeEach } from 'vitest'
import React from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { AICollectionFiller } from './AICollectionFiller'
import type { ExtractedCollectionData } from '@/lib/types/extraction'

// ──────────────────────────────────────────────
// Mocks
// ──────────────────────────────────────────────

let mockExtractResult: { data?: ExtractedCollectionData; warnings?: string[] } = {
  data: undefined,
  warnings: undefined,
}
let mockExtractIsPending = false
let mockExtractError: Error | null = null
let mockExtractCalls: Array<unknown> = []

vi.mock('../hooks', () => ({
  useCollectionExtraction: () => ({
    mutate: (
      input: unknown,
      opts?: { onSuccess?: (data: typeof mockExtractResult) => void }
    ) => {
      mockExtractCalls.push(input)
      opts?.onSuccess?.(mockExtractResult)
    },
    isPending: mockExtractIsPending,
    error: mockExtractError,
    reset: vi.fn(),
  }),
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    onClick,
    disabled,
    ...props
  }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button onClick={onClick} disabled={disabled} {...(props as Record<string, unknown>)}>
      {children}
    </button>
  ),
}))

vi.mock('@/components/ui/badge', () => ({
  Badge: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}))

vi.mock('@/components/shared', () => ({
  InlineErrorBanner: ({
    children,
    testId,
  }: {
    children: React.ReactNode
    testId?: string
  }) => <div data-testid={testId}>{children}</div>,
}))

// ──────────────────────────────────────────────
// Component
// ──────────────────────────────────────────────

describe('AICollectionFiller', () => {
  beforeEach(() => {
    mockExtractResult = { data: undefined, warnings: undefined }
    mockExtractIsPending = false
    mockExtractError = null
    mockExtractCalls = []
  })

  it('renders textarea + image drop zone + disabled extract button', () => {
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    expect(screen.getByTestId('ai-collection-filler-textarea')).toBeInTheDocument()
    expect(screen.getByTestId('ai-collection-filler-file-input')).toBeInTheDocument()
    expect(screen.getByTestId('ai-collection-filler-extract')).toBeDisabled()
  })

  it('enables Extract once text is typed', async () => {
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      '1. Kendrick Lamar — TPAB'
    )
    expect(screen.getByTestId('ai-collection-filler-extract')).not.toBeDisabled()
  })

  it('file input accepts HEIC + HEIF for iOS Safari paste', () => {
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    const input = screen.getByTestId('ai-collection-filler-file-input') as HTMLInputElement
    expect(input.getAttribute('accept')).toContain('image/heic')
    expect(input.getAttribute('accept')).toContain('image/heif')
    // The .heic/.heif extension fallbacks let Safari's photo picker show
    // files that report an empty `file.type` (caught: iOS Safari sometimes
    // drops the mime-type on clipboard paste).
    expect(input.getAttribute('accept')).toContain('.heic')
  })

  it('renders MATCH / PICK / NEW badges after extraction', async () => {
    mockExtractResult = {
      data: {
        source: 'The 200 Best Albums of the 2010s',
        items: [
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'To Pimp a Butterfly',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
            matched_artist_slug: 'kendrick-lamar',
          },
          {
            artist_name: 'Boris',
            release_title: 'Pink',
            artist_suggestions: [
              { id: 100, name: 'Boris', slug: 'boris' },
              { id: 101, name: 'Boris (US)', slug: 'boris-us' },
            ],
          },
          {
            artist_name: 'Some Made Up Band',
            release_title: 'Nowhere Album',
          },
        ],
      },
    }
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      'list goes here'
    )
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    expect(screen.getByText('MATCH')).toBeInTheDocument()
    expect(screen.getByText('PICK')).toBeInTheDocument()
    expect(screen.getByText('NEW')).toBeInTheDocument()
    expect(screen.getByText('The 200 Best Albums of the 2010s')).toBeInTheDocument()
  })

  it('clicking Add on a MATCH row stages exactly that row', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'To Pimp a Butterfly',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
            matched_artist_slug: 'kendrick-lamar',
          },
        ],
      },
    }
    const onStage = vi.fn()
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={onStage} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    await user.click(screen.getByTestId('ai-collection-filler-row-add'))

    // Single batched call carrying the matched entity_id.
    expect(onStage).toHaveBeenCalledTimes(1)
    expect(onStage.mock.calls[0][0]).toEqual([
      expect.objectContaining({
        entityType: 'artist',
        entityId: 42,
        name: 'Kendrick Lamar',
      }),
    ])
    // Subtitle is the raw release title — StagedRow inserts its own ' — '
    // separator, so prepending one here would render a doubled em-dash.
    expect(onStage.mock.calls[0][0][0].subtitle).toBe('To Pimp a Butterfly')
  })

  // Regression: canon lists ("100 Best Albums") contain multiple releases
  // by the same artist. Without per-batch dedup, all rows collapsing to
  // one matched_artist_id would emit duplicate-key React warnings and the
  // backend's UNIQUE(collection_id, entity_type, entity_id) would silently
  // keep only one.
  it('Add all matched deduplicates same-artist rows within the batch', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'To Pimp a Butterfly',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'DAMN.',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
          {
            artist_name: 'Frank Ocean',
            release_title: 'Blonde',
            matched_artist_id: 43,
            matched_artist_name: 'Frank Ocean',
          },
        ],
      },
    }
    const onStage = vi.fn()
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={onStage} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    await user.click(screen.getByTestId('ai-collection-filler-add-all-matched'))

    expect(onStage).toHaveBeenCalledTimes(1)
    const batched = onStage.mock.calls[0][0] as Array<{ entityId: number }>
    expect(batched).toHaveLength(2)
    expect(batched.map(b => b.entityId).sort()).toEqual([42, 43])
  })

  // Regression: handleTextChange must preserve extractionResult across
  // keystrokes — each row can take multiple manual interactions (Pick
  // suggestions, accept-then-skip cycles). Wiping on every character
  // would force users to re-extract from scratch.
  it('typing in the textarea after extraction preserves the extracted rows', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
        ],
      },
    }
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    expect(screen.getByText('Kendrick Lamar')).toBeInTheDocument()

    // Typing another character must NOT wipe the result.
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), '!')
    expect(screen.getByText('Kendrick Lamar')).toBeInTheDocument()
  })

  it('accepting a "Did you mean" suggestion promotes the row to MATCH', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Boris',
            release_title: 'Pink',
            artist_suggestions: [
              { id: 100, name: 'Boris', slug: 'boris' },
              { id: 101, name: 'Boris (US)', slug: 'boris-us' },
            ],
          },
        ],
      },
    }
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    expect(screen.getByText('PICK')).toBeInTheDocument()
    // Click the first suggestion chip.
    const suggestionButtons = screen.getAllByTestId('ai-collection-filler-row-pick')
    await user.click(suggestionButtons[0])

    // PICK badge gone; MATCH badge present.
    expect(screen.queryByText('PICK')).not.toBeInTheDocument()
    expect(screen.getByText('MATCH')).toBeInTheDocument()
  })

  it('"Add all matched" stages all eligible rows in one batched call', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
          {
            artist_name: 'Frank Ocean',
            matched_artist_id: 43,
            matched_artist_name: 'Frank Ocean',
          },
          {
            // Suggestion-only row — should NOT be in the batch.
            artist_name: 'Boris',
            artist_suggestions: [{ id: 100, name: 'Boris', slug: 'boris' }],
          },
        ],
      },
    }
    const onStage = vi.fn()
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={onStage} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    await user.click(screen.getByTestId('ai-collection-filler-add-all-matched'))

    // ONE call (batched) carrying both matched entities, NOT the suggestion-only row.
    expect(onStage).toHaveBeenCalledTimes(1)
    const batched = onStage.mock.calls[0][0] as Array<{ entityId: number }>
    expect(batched).toHaveLength(2)
    expect(batched.map(b => b.entityId)).toEqual([42, 43])
  })

  it('already-staged rows render an "Added" chip instead of [Add]', async () => {
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
        ],
      },
    }
    const user = userEvent.setup()
    render(
      <AICollectionFiller
        onStageItems={vi.fn()}
        alreadyStaged={(t, id) => t === 'artist' && id === 42}
      />
    )
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    expect(screen.getByText('Added')).toBeInTheDocument()
    expect(screen.queryByTestId('ai-collection-filler-row-add')).not.toBeInTheDocument()
  })

  it('surfaces a hook error via InlineErrorBanner', async () => {
    mockExtractError = new Error('AI service temporarily unavailable.')
    const user = userEvent.setup()
    render(<AICollectionFiller onStageItems={vi.fn()} alreadyStaged={() => false} />)
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')

    const banner = screen.getByTestId('ai-collection-filler-error')
    expect(banner).toHaveTextContent(/temporarily unavailable/i)
  })
})
