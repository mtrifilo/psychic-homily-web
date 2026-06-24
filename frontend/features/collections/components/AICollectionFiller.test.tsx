import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import React from 'react'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { AICollectionFiller } from './AICollectionFiller'
import type { AICollectionFillerProps } from './AICollectionFiller'
import type {
  ExtractCollectionRequest,
  ExtractedCollectionData,
} from '@/lib/types/extraction'

// jsdom does not fire image load events on `src` assignment, and its canvas
// has no 2d context — both of which compressImage() depends on. These stubs
// drive the happy path (onload → real getContext → toDataURL produces the
// compressed JPEG preview) so the image-extract request shape can be asserted.
// The decode-failure path overrides Image with one that fires `onerror`.
const COMPRESSED_JPEG_DATA_URL =
  'data:image/jpeg;base64,Y29tcHJlc3NlZA==' // "compressed"

class MockImageOnLoad {
  onload: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  width = 800
  height = 600
  private _src = ''
  set src(value: string) {
    this._src = value
    queueMicrotask(() => this.onload?.())
  }
  get src() {
    return this._src
  }
}

class MockImageOnError {
  onload: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  width = 800
  height = 600
  private _src = ''
  set src(value: string) {
    this._src = value
    queueMicrotask(() => this.onerror?.(new Error('decode failed')))
  }
  get src() {
    return this._src
  }
}

/**
 * Stub canvas so compressImage() resolves to a known JPEG data URL. jsdom's
 * getContext returns null (compressImage rejects on that), so without this the
 * happy image path would never produce a preview.
 */
function stubCanvasCompression() {
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    // compressImage() calls ctx.drawImage before reading toDataURL — a bare {}
    // ctx makes it throw "drawImage is not a function".
    drawImage: vi.fn(),
  } as unknown as CanvasRenderingContext2D)
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue(
    COMPRESSED_JPEG_DATA_URL
  )
}

/** Upload a file via the hidden file input, returning once handled. */
async function uploadFile(
  user: ReturnType<typeof userEvent.setup>,
  file: File
) {
  const input = screen.getByTestId(
    'ai-collection-filler-file-input'
  ) as HTMLInputElement
  await user.upload(input, file)
}

/**
 * Set a file on the input via fireEvent — bypasses userEvent's
 * accept-attribute filter, which silently refuses files whose type isn't in
 * the input's `accept` list. The component's own MIME guard (and the
 * drag-drop path in production) handle unsupported types regardless of
 * `accept`, so this drives that guard directly.
 */
function changeFile(file: File) {
  const input = screen.getByTestId(
    'ai-collection-filler-file-input'
  ) as HTMLInputElement
  fireEvent.change(input, { target: { files: [file] } })
}

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

// Tier-gated affordances read user.is_admin + user.user_tier from auth context.
let mockUser: { is_admin?: boolean; user_tier?: string } | null = null
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: mockUser }),
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
  // Forward arbitrary props (incl. data-testid) so chip testids survive the
  // mock — the real Badge spreads props onto its root element too.
  Badge: ({
    children,
    ...props
  }: {
    children: React.ReactNode
    [key: string]: unknown
  }) => <span {...(props as Record<string, unknown>)}>{children}</span>,
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

// Wrap renders in a QueryClientProvider — the component now always mounts a
// local useMutation (queue-create), which needs a QueryClient even in tests
// where the queue path isn't exercised.
function renderFiller(props: AICollectionFillerProps) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AICollectionFiller {...props} />
    </QueryClientProvider>
  )
}

describe('AICollectionFiller', () => {
  beforeEach(() => {
    mockExtractResult = { data: undefined, warnings: undefined }
    mockExtractIsPending = false
    mockExtractError = null
    mockExtractCalls = []
    mockUser = null
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders textarea + image drop zone + disabled extract button', () => {
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
    expect(screen.getByTestId('ai-collection-filler-textarea')).toBeInTheDocument()
    expect(screen.getByTestId('ai-collection-filler-file-input')).toBeInTheDocument()
    expect(screen.getByTestId('ai-collection-filler-extract')).toBeDisabled()
  })

  it('enables Extract once text is typed', async () => {
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      '1. Kendrick Lamar — TPAB'
    )
    expect(screen.getByTestId('ai-collection-filler-extract')).not.toBeDisabled()
  })

  it('file input accepts HEIC + HEIF for iOS Safari paste', () => {
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
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
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
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
    renderFiller({ onStageItems: onStage, alreadyStaged: () => false })
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
    renderFiller({ onStageItems: onStage, alreadyStaged: () => false })
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
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
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
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
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
    renderFiller({ onStageItems: onStage, alreadyStaged: () => false })
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
    renderFiller({
      onStageItems: vi.fn(),
      alreadyStaged: (t, id) => t === 'artist' && id === 42,
    })
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    expect(screen.getByText('Added')).toBeInTheDocument()
    expect(screen.queryByTestId('ai-collection-filler-row-add')).not.toBeInTheDocument()
  })

  it('surfaces a hook error via InlineErrorBanner', async () => {
    mockExtractError = new Error('AI service temporarily unavailable.')
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')

    const banner = screen.getByTestId('ai-collection-filler-error')
    expect(banner).toHaveTextContent(/temporarily unavailable/i)
  })

  // ────────────────────────────────────────────────────────────
  // PSY-853: tier-gated create / queue actions on unmatched (NEW) rows
  // ────────────────────────────────────────────────────────────

  // One unmatched (NEW) row — no match, no suggestions — so the tier-gated
  // create/queue affordance is the only action on the row.
  const ONE_UNMATCHED_ROW: ExtractedCollectionData = {
    items: [{ artist_name: 'Some Made Up Band', release_title: 'Nowhere Album' }],
  }

  /**
   * Stub global.fetch with a single JSON response for the entity-request POST.
   * createdEntityId (PSY-1008) is included when provided — that's the
   * auto-approve-fulfilled path that triggers inline create-and-add.
   */
  function stubFetch(
    decisionState: 'approved' | 'pending',
    ok = true,
    createdEntityId?: number
  ) {
    const fetchMock = vi.fn().mockResolvedValue({
      ok,
      json: async () => ({
        id: 7,
        decision_state: decisionState,
        ...(createdEntityId !== undefined
          ? { created_entity_id: createdEntityId }
          : {}),
      }),
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  async function extractOneUnmatchedRow(
    user: ReturnType<typeof userEvent.setup>,
    onStage: AICollectionFillerProps['onStageItems'] = vi.fn()
  ) {
    mockExtractResult = { data: ONE_UNMATCHED_ROW }
    renderFiller({ onStageItems: onStage, alreadyStaged: () => false })
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))
    return onStage
  }

  it('admin sees [Create + Add] on an unmatched row (no Queue button)', async () => {
    mockUser = { is_admin: true, user_tier: 'trusted_contributor' }
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    const btn = screen.getByTestId('ai-collection-filler-row-request')
    expect(btn).toHaveTextContent('Create + Add')
    expect(btn).not.toHaveTextContent('Queue for review')
  })

  it('local_ambassador sees [Create + Add] (auto-approve tier, not admin)', async () => {
    mockUser = { is_admin: false, user_tier: 'local_ambassador' }
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    expect(
      screen.getByTestId('ai-collection-filler-row-request')
    ).toHaveTextContent('Create + Add')
  })

  it('contributor sees [Queue for review] (never an inline create)', async () => {
    mockUser = { is_admin: false, user_tier: 'contributor' }
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    const btn = screen.getByTestId('ai-collection-filler-row-request')
    expect(btn).toHaveTextContent('Queue for review')
    expect(btn).not.toHaveTextContent('Create + Add')
  })

  it('new_user sees [Queue for review]', async () => {
    mockUser = { is_admin: false, user_tier: 'new_user' }
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    expect(
      screen.getByTestId('ai-collection-filler-row-request')
    ).toHaveTextContent('Queue for review')
  })

  it('anonymous / unknown tier sees NO create or queue action', async () => {
    mockUser = null
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    expect(
      screen.queryByTestId('ai-collection-filler-row-request')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('ai-collection-filler-row-confirm')
    ).not.toBeInTheDocument()
  })

  it('trusted_contributor requires inline [Confirm] before filing', async () => {
    mockUser = { is_admin: false, user_tier: 'trusted_contributor' }
    const fetchMock = stubFetch('approved')
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    // First click reveals Confirm/Cancel — it does NOT file the request.
    await user.click(screen.getByTestId('ai-collection-filler-row-request'))
    expect(fetchMock).not.toHaveBeenCalled()
    expect(
      screen.getByTestId('ai-collection-filler-row-confirm')
    ).toBeInTheDocument()
    expect(
      screen.getByTestId('ai-collection-filler-row-cancel')
    ).toBeInTheDocument()

    // Cancel backs out without filing.
    await user.click(screen.getByTestId('ai-collection-filler-row-cancel'))
    expect(fetchMock).not.toHaveBeenCalled()
    expect(
      screen.getByTestId('ai-collection-filler-row-request')
    ).toBeInTheDocument()

    // Re-open and Confirm files a confirmed request.
    await user.click(screen.getByTestId('ai-collection-filler-row-request'))
    await user.click(screen.getByTestId('ai-collection-filler-row-confirm'))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/entity-requests')
    const body = JSON.parse((init as RequestInit).body as string)
    expect(body.confirmed).toBe(true)
    expect(body.entity_type).toBe('artist')
    expect(body.source_context).toBe('ai_extraction')
    expect(body.payload).toEqual({ name: 'Some Made Up Band' })
  })

  it('queue path POSTs an entity_request and shows a "Queued" chip', async () => {
    mockUser = { is_admin: false, user_tier: 'contributor' }
    const fetchMock = stubFetch('pending')
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    // Sanity: the unmatched row rendered before we act on it.
    expect(screen.getByText('Some Made Up Band')).toBeInTheDocument()

    await user.click(screen.getByTestId('ai-collection-filler-row-request'))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/entity-requests')
    const body = JSON.parse((init as RequestInit).body as string)
    expect(body.confirmed).toBe(false)
    expect(body.source_context).toBe('ai_extraction')

    // Pending decision_state → "Queued" chip; create/queue button replaced.
    const chip = await screen.findByTestId(
      'ai-collection-filler-row-request-chip'
    )
    expect(chip).toHaveTextContent('Queued')
    expect(
      screen.queryByTestId('ai-collection-filler-row-request')
    ).not.toBeInTheDocument()
  })

  it('auto-approved create-and-add stages the created entity and shows "Added"', async () => {
    mockUser = { is_admin: true, user_tier: 'trusted_contributor' }
    // PSY-1008: auto-approve fulfills the entity and returns created_entity_id.
    stubFetch('approved', true, 99)
    const user = userEvent.setup()
    const onStage = await extractOneUnmatchedRow(user)

    await user.click(screen.getByTestId('ai-collection-filler-row-request'))

    // The newly created entity is staged into the collection (true create-and-add).
    await waitFor(() =>
      expect(onStage).toHaveBeenCalledWith([
        {
          entityType: 'artist',
          entityId: 99,
          name: 'Some Made Up Band',
          subtitle: null,
        },
      ])
    )
    const chip = await screen.findByTestId(
      'ai-collection-filler-row-request-chip'
    )
    expect(chip).toHaveTextContent('Added')
  })

  it('auto-approved WITHOUT created_entity_id (deferred fulfillment) shows "Requested" and does not stage', async () => {
    mockUser = { is_admin: true, user_tier: 'trusted_contributor' }
    stubFetch('approved') // approved but not fulfilled — no created_entity_id
    const user = userEvent.setup()
    const onStage = await extractOneUnmatchedRow(user)

    await user.click(screen.getByTestId('ai-collection-filler-row-request'))

    const chip = await screen.findByTestId(
      'ai-collection-filler-row-request-chip'
    )
    expect(chip).toHaveTextContent('Requested')
    expect(onStage).not.toHaveBeenCalled()
  })

  it('a failed entity-request shows an inline error and keeps the button', async () => {
    mockUser = { is_admin: false, user_tier: 'contributor' }
    // 403 (or any non-ok) → the mutationFn throws; the row surfaces it inline.
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      json: async () => ({ message: 'Admin access required' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    await user.click(screen.getByTestId('ai-collection-filler-row-request'))

    const err = await screen.findByTestId(
      'ai-collection-filler-row-request-error'
    )
    expect(err).toHaveTextContent('Admin access required')
    // No chip; the create/queue button remains so the user can retry.
    expect(
      screen.queryByTestId('ai-collection-filler-row-request-chip')
    ).not.toBeInTheDocument()
    expect(
      screen.getByTestId('ai-collection-filler-row-request')
    ).toBeInTheDocument()
  })

  it('does not double-file when the create button is clicked twice fast', async () => {
    mockUser = { is_admin: true, user_tier: 'trusted_contributor' }
    // Never-resolving fetch keeps the request in flight so a second click
    // would double-file if the in-flight guard were missing.
    const fetchMock = vi.fn().mockReturnValue(new Promise(() => {}))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    await extractOneUnmatchedRow(user)

    const btn = screen.getByTestId('ai-collection-filler-row-request')
    await user.click(btn)
    // Button is now disabled (in flight) — a second click must be a no-op.
    await user.click(btn)

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('matched-row [Add] still works when a tier user is present (no regress)', async () => {
    mockUser = { is_admin: true, user_tier: 'trusted_contributor' }
    mockExtractResult = {
      data: {
        items: [
          {
            artist_name: 'Kendrick Lamar',
            release_title: 'To Pimp a Butterfly',
            matched_artist_id: 42,
            matched_artist_name: 'Kendrick Lamar',
          },
        ],
      },
    }
    const onStage = vi.fn()
    const user = userEvent.setup()
    renderFiller({ onStageItems: onStage, alreadyStaged: () => false })
    await user.type(screen.getByTestId('ai-collection-filler-textarea'), 'list')
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    await user.click(screen.getByTestId('ai-collection-filler-row-add'))
    expect(onStage).toHaveBeenCalledTimes(1)
    expect(onStage.mock.calls[0][0][0]).toEqual(
      expect.objectContaining({ entityType: 'artist', entityId: 42 })
    )
    // A matched row never shows the create/queue affordance.
    expect(
      screen.queryByTestId('ai-collection-filler-row-request')
    ).not.toBeInTheDocument()
  })

  // ────────────────────────────────────────────────────────────
  // PSY-857: image-only + image+text extract modes (request `type`)
  //
  // handleExtract branches on hasText/hasImage to set type='text'|'image'|
  // 'both' and to attach image_data/text. The V1 suite only covered the
  // 'text' branch (it never uploaded a file); these assert the image branches
  // send the right request body via the mocked hook's captured `mutate` arg.
  // ────────────────────────────────────────────────────────────

  /** Last request body the component passed to the (mocked) extraction hook. */
  function lastExtractRequest(): ExtractCollectionRequest {
    return mockExtractCalls.at(-1) as ExtractCollectionRequest
  }

  it('image-only upload extracts with type "image" and the base64 image_data', async () => {
    stubCanvasCompression()
    vi.stubGlobal('Image', MockImageOnLoad)
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })

    const file = new File([new Uint8Array([1, 2, 3, 4])], 'list.jpg', {
      type: 'image/jpeg',
    })
    await uploadFile(user, file)

    // Wait for compressImage to resolve → preview visible → Extract enabled.
    await waitFor(() =>
      expect(
        screen.getByAltText('Uploaded article screenshot')
      ).toBeInTheDocument()
    )
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    const req = lastExtractRequest()
    expect(req.type).toBe('image')
    // image_data is the base64 payload AFTER the "data:image/jpeg;base64,"
    // prefix is stripped (the component splits on ',').
    expect(req.image_data).toBe(COMPRESSED_JPEG_DATA_URL.split(',')[1])
    expect(req.media_type).toBe('image/jpeg')
    // type 'image' carries no text.
    expect(req.text).toBeUndefined()
  })

  it('image + text upload extracts with type "both" carrying text AND image_data', async () => {
    stubCanvasCompression()
    vi.stubGlobal('Image', MockImageOnLoad)
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })

    const file = new File([new Uint8Array([5, 6, 7, 8])], 'list.png', {
      type: 'image/png',
    })
    await uploadFile(user, file)
    await waitFor(() =>
      expect(
        screen.getByAltText('Uploaded article screenshot')
      ).toBeInTheDocument()
    )

    // Add context text alongside the image → 'both'.
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      'Pitchfork best of 2010s'
    )
    await user.click(screen.getByTestId('ai-collection-filler-extract'))

    const req = lastExtractRequest()
    expect(req.type).toBe('both')
    expect(req.text).toBe('Pitchfork best of 2010s')
    expect(req.image_data).toBe(COMPRESSED_JPEG_DATA_URL.split(',')[1])
    // media_type is always image/jpeg — compressImage re-encodes to JPEG.
    expect(req.media_type).toBe('image/jpeg')
  })

  // ────────────────────────────────────────────────────────────
  // PSY-857: image-input error paths (size / unsupported type / decode fail)
  // ────────────────────────────────────────────────────────────

  it('rejects an oversized image with the 10MB size error', async () => {
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })

    // 11MB > MAX_IMAGE_SIZE (10MB). A real Uint8Array that large is slow to
    // allocate in jsdom; spoof `size` via Object.defineProperty instead.
    const file = new File([new Uint8Array([1])], 'huge.jpg', {
      type: 'image/jpeg',
    })
    Object.defineProperty(file, 'size', { value: 11 * 1024 * 1024 })
    await uploadFile(user, file)

    const banner = await screen.findByTestId(
      'ai-collection-filler-image-error'
    )
    expect(banner).toHaveTextContent(/too large/i)
    expect(banner).toHaveTextContent(/10MB/i)
    // No preview rendered for a rejected file.
    expect(
      screen.queryByAltText('Uploaded article screenshot')
    ).not.toBeInTheDocument()
  })

  it('rejects an unsupported MIME type with the supported-types error', async () => {
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })

    const file = new File([new Uint8Array([1, 2])], 'doc.pdf', {
      type: 'application/pdf',
    })
    changeFile(file)

    const banner = await screen.findByTestId(
      'ai-collection-filler-image-error'
    )
    expect(banner).toHaveTextContent(/unsupported image type/i)
    // Copy lists the accepted types.
    expect(banner).toHaveTextContent(/JPEG, PNG, GIF, WebP, or HEIC/i)
    expect(
      screen.queryByAltText('Uploaded article screenshot')
    ).not.toBeInTheDocument()
  })

  it('clears the file + preview and shows an error when image decode fails', async () => {
    // Image fires onerror → compressImage rejects → component sets imageError
    // and clears BOTH imageFile and imagePreview so the dropzone returns.
    vi.stubGlobal('Image', MockImageOnError)
    const user = userEvent.setup()
    renderFiller({ onStageItems: vi.fn(), alreadyStaged: () => false })

    const file = new File([new Uint8Array([1, 2, 3, 4])], 'broken.heic', {
      type: 'image/heic',
    })
    await uploadFile(user, file)

    const banner = await screen.findByTestId(
      'ai-collection-filler-image-error'
    )
    // compressImage's onerror reject message mentions HEIC/iOS Safari guidance.
    expect(banner).toHaveTextContent(/failed to decode image/i)

    // imagePreview cleared → the dropzone (file input) is back, no preview img.
    expect(
      screen.queryByAltText('Uploaded article screenshot')
    ).not.toBeInTheDocument()
    expect(
      screen.getByTestId('ai-collection-filler-file-input')
    ).toBeInTheDocument()

    // imageFile cleared (not merely imagePreview): if imageFile were stranded
    // non-null with imagePreview null, `imageReady` would stay false and
    // Extract would never enable — even after typing text. Proving that typing
    // text RE-ENABLES Extract pins imageFile === null, which a bare
    // "Extract is disabled" assertion can't distinguish from the stranded case.
    expect(screen.getByTestId('ai-collection-filler-extract')).toBeDisabled()
    await user.type(
      screen.getByTestId('ai-collection-filler-textarea'),
      'fallback text'
    )
    expect(
      screen.getByTestId('ai-collection-filler-extract')
    ).not.toBeDisabled()
  })
})
