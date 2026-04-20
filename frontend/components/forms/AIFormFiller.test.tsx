import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AIFormFiller } from './AIFormFiller'
import type {
  ExtractedShowData,
  ExtractShowResponse,
} from '@/lib/types/extraction'

// Controllable mock for useShowExtraction. This replaces the raw fetch in the
// underlying hook so no network layer (MSW or otherwise) is needed.
type MockHookState = {
  mutate: (
    req: unknown,
    opts: { onSuccess?: (response: ExtractShowResponse) => void }
  ) => void
  isPending: boolean
  error: Error | null
  reset: () => void
}

const mockHookState: MockHookState = {
  mutate: vi.fn(),
  isPending: false,
  error: null,
  reset: vi.fn(),
}

vi.mock('@/features/shows', () => ({
  useShowExtraction: () => mockHookState,
}))

const MOCK_EXTRACTED_DATA: ExtractedShowData = {
  artists: [
    {
      name: 'The National',
      is_headliner: true,
      matched_id: 1,
      matched_name: 'The National',
      matched_slug: 'the-national',
    },
  ],
  venue: {
    name: 'Valley Bar',
    city: 'Phoenix',
    state: 'AZ',
    matched_id: 1,
    matched_name: 'Valley Bar',
    matched_slug: 'valley-bar-phoenix-az',
  },
  date: '2026-03-15',
  time: '20:00',
  cost: '$35',
  ages: '21+',
}

// jsdom does not fire image load events when setting src. The component's
// compressImage() awaits onload before resolving the preview. Stub Image so
// onload fires on the microtask queue after src assignment. Canvas.getContext
// still returns null in jsdom, which makes compressImage() reject; the
// component catches that and falls back to the original data URL — both
// branches end up displaying the preview img.
class MockImage {
  onload: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  width = 100
  height = 100
  private _src = ''
  set src(value: string) {
    this._src = value
    queueMicrotask(() => this.onload?.())
  }
  get src() {
    return this._src
  }
}

describe('AIFormFiller', () => {
  beforeEach(() => {
    mockHookState.mutate = vi.fn()
    mockHookState.isPending = false
    mockHookState.error = null
    mockHookState.reset = vi.fn()
    vi.stubGlobal('Image', MockImage)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('extracts show info from pasted text and fires onExtracted', async () => {
    mockHookState.mutate = vi.fn((_req, opts) => {
      opts.onSuccess?.({
        success: true,
        data: MOCK_EXTRACTED_DATA,
        warnings: [],
      })
    })

    const onExtracted = vi.fn()
    const user = userEvent.setup()
    render(<AIFormFiller onExtracted={onExtracted} />)

    // Expand the AI card
    await user.click(screen.getByText('AI Form Filler-Outer'))

    // Type flyer text into the AI textarea
    await user.type(
      screen.getByPlaceholderText(/Paste show details/),
      'The National at Valley Bar'
    )

    // Click extract
    await user.click(
      screen.getByRole('button', { name: /Extract Show Info/ })
    )

    // Assert extraction complete + extracted badges visible
    expect(await screen.findByText('Extraction Complete')).toBeInTheDocument()
    expect(screen.getByText('The National')).toBeInTheDocument()
    expect(screen.getByText('Valley Bar')).toBeInTheDocument()

    // onExtracted callback fired with the extracted data (this is what the
    // parent ShowForm uses to populate venue.city / date fields in the E2E).
    expect(onExtracted).toHaveBeenCalledWith(MOCK_EXTRACTED_DATA)
  })

  it('extracts show info from an uploaded image and shows preview', async () => {
    mockHookState.mutate = vi.fn((_req, opts) => {
      opts.onSuccess?.({
        success: true,
        data: MOCK_EXTRACTED_DATA,
        warnings: [],
      })
    })

    const onExtracted = vi.fn()
    const user = userEvent.setup()
    const { container } = render(<AIFormFiller onExtracted={onExtracted} />)

    // Expand the AI card
    await user.click(screen.getByText('AI Form Filler-Outer'))

    // Upload a fake flyer image. Content is arbitrary — FileReader produces a
    // data URL; the component's compressImage falls back to the original when
    // the jsdom canvas returns no 2d context.
    const fileInput = container.querySelector(
      'input[type="file"]'
    ) as HTMLInputElement
    const fakeFile = new File([new Uint8Array([1, 2, 3, 4])], 'flyer.png', {
      type: 'image/png',
    })
    await user.upload(fileInput, fakeFile)

    // Assert image preview appears
    await waitFor(() =>
      expect(
        screen.getByAltText('Uploaded flyer')
      ).toBeInTheDocument()
    )

    // Click extract
    await user.click(
      screen.getByRole('button', { name: /Extract Show Info/ })
    )

    // Assert extraction complete + onExtracted fired
    expect(await screen.findByText('Extraction Complete')).toBeInTheDocument()
    expect(onExtracted).toHaveBeenCalledWith(MOCK_EXTRACTED_DATA)
  })

  it('renders an error alert when extraction fails', async () => {
    // Simulate a failed mutation: useShowExtraction surfaces the thrown error
    // via its `error` field. The component renders its message in a
    // destructive Alert. (In the real flow the error is set by React Query
    // after mutationFn throws; here we set it directly since the hook is
    // mocked — the rendering behavior is identical.)
    mockHookState.error = new Error('AI service is temporarily unavailable')

    const user = userEvent.setup()
    render(<AIFormFiller onExtracted={vi.fn()} />)

    // Expand the AI card so the error region is rendered.
    await user.click(screen.getByText('AI Form Filler-Outer'))

    // Assert error alert visible.
    expect(
      screen.getByText('AI service is temporarily unavailable')
    ).toBeInTheDocument()
  })
})
