import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { fireEvent } from '@testing-library/dom'
import { ShareButton, buildShareUrl } from './ShareButton'

const SHOW_PATH = '/shows/some-show'
const SHOW_URL = 'https://psychichomily.com/shows/some-show'

/**
 * jsdom ships neither `navigator.share` nor `navigator.clipboard`, and the
 * component's whole contract is "what does this browser actually support", so
 * every test states its capabilities explicitly rather than inheriting whatever
 * a previous test left behind.
 */
function setCapabilities({
  share,
  writeText,
}: {
  share?: (data: ShareData) => Promise<void>
  writeText?: (text: string) => Promise<void>
}) {
  for (const [key, value] of [
    ['share', share],
    ['clipboard', writeText ? { writeText } : undefined],
  ] as const) {
    Object.defineProperty(navigator, key, {
      value,
      configurable: true,
      writable: true,
    })
  }
}

beforeEach(() => {
  setCapabilities({})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('buildShareUrl', () => {
  it('builds a canonical apex URL from a path', () => {
    expect(buildShareUrl(SHOW_PATH)).toBe(SHOW_URL)
  })

  it('strips campaign tags and fragments so user shares are never tagged', () => {
    expect(
      buildShareUrl('/shows/some-show?utm_source=bluesky&utm_campaign=x')
    ).toBe(SHOW_URL)
    expect(buildShareUrl('/artists/foo#graph')).toBe(
      'https://psychichomily.com/artists/foo'
    )
  })

  it('cannot be redirected to another origin', () => {
    expect(buildShareUrl('https://evil.example/x')).toBe(
      'https://psychichomily.com/x'
    )
    expect(buildShareUrl('//evil.example')).toBe('https://psychichomily.com/')
  })

  it('percent-encodes a path that is not already URL-safe', () => {
    expect(buildShareUrl('/artists/bad slug')).toBe(
      'https://psychichomily.com/artists/bad%20slug'
    )
  })
})

describe('ShareButton — Web Share API path', () => {
  it('opens the native share sheet with the canonical URL and no composed text', async () => {
    const share = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ share })

    render(<ShareButton path={SHOW_PATH} />)
    const button = await screen.findByRole('button', { name: 'Share' })
    fireEvent.click(button)

    await waitFor(() => expect(share).toHaveBeenCalledTimes(1))
    // URL only: no `title`, no `text`. The OG card carries the context, and
    // nothing entity-derived is written into the payload.
    expect(share).toHaveBeenCalledWith({ url: SHOW_URL })
  })

  it('shows no inline confirmation after a successful native share', async () => {
    const share = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ share })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(share).toHaveBeenCalled())
    // The OS sheet is its own feedback; a "Copied" badge here would be a lie.
    expect(screen.queryByText('Copied')).not.toBeInTheDocument()
    expect(screen.queryByText('Copy failed')).not.toBeInTheDocument()
  })

  it('prefers the share sheet over the clipboard when both exist', async () => {
    const share = vi.fn().mockResolvedValue(undefined)
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ share, writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(share).toHaveBeenCalled())
    expect(writeText).not.toHaveBeenCalled()
  })
})

describe('ShareButton — AbortError dismissal', () => {
  it('treats a dismissed share sheet as normal, not a failure', async () => {
    // A real browser rejects with a DOMException here, not an Error.
    const share = vi
      .fn()
      .mockRejectedValue(new DOMException('Share canceled', 'AbortError'))
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ share, writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(share).toHaveBeenCalled())
    expect(screen.queryByText('Copy failed')).not.toBeInTheDocument()
    expect(screen.queryByText('Copied')).not.toBeInTheDocument()
    // Dismissing means "I decided not to share" — silently copying anyway
    // would put a link on the clipboard the user never asked for.
    expect(writeText).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Share' })).toHaveTextContent(
      'Share'
    )
  })

  it('recognises an AbortError that does not inherit from Error', async () => {
    // Guards the duck-typed check: engines where `DOMException` is not an
    // `Error` subclass must still take the silent path.
    const share = vi.fn().mockRejectedValue({ name: 'AbortError' })
    setCapabilities({ share })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(share).toHaveBeenCalled())
    expect(screen.queryByText('Copy failed')).not.toBeInTheDocument()
  })

  it('falls back to the clipboard when the sheet fails for any other reason', async () => {
    const share = vi
      .fn()
      .mockRejectedValue(new DOMException('no target', 'NotAllowedError'))
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ share, writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(SHOW_URL))
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })
})

describe('ShareButton — clipboard fallback path', () => {
  it('copies the canonical URL and confirms inline when there is no share sheet', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(SHOW_URL))
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('copies the canonical URL, not the current location', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })

    render(<ShareButton path="/scenes/chicago-il/2026-W31" />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        'https://psychichomily.com/scenes/chicago-il/2026-W31'
      )
    )
    expect(writeText).not.toHaveBeenCalledWith(
      expect.stringContaining(window.location.origin)
    )
  })

  it('reports an honest failure when the clipboard write rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    setCapabilities({ writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))

    expect(await screen.findByText('Copy failed')).toBeInTheDocument()
  })

  it('reverts to the idle label after the confirmation window', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })

    render(<ShareButton path={SHOW_PATH} />)
    fireEvent.click(await screen.findByRole('button', { name: 'Share' }))
    expect(await screen.findByText('Copied')).toBeInTheDocument()

    await vi.advanceTimersByTimeAsync(2100)
    await waitFor(() =>
      expect(screen.queryByText('Copied')).not.toBeInTheDocument()
    )
    vi.useRealTimers()
  })
})

describe('ShareButton — never a dead control', () => {
  it('renders nothing when neither the share sheet nor the clipboard exists', async () => {
    setCapabilities({})
    const { container } = render(<ShareButton path={SHOW_PATH} />)

    // Capability resolves in an effect, so assert after a flush rather than
    // on the first paint.
    await waitFor(() => expect(container).toBeEmptyDOMElement())
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('appears once a usable capability is detected', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })
    render(<ShareButton path={SHOW_PATH} />)
    expect(
      await screen.findByRole('button', { name: 'Share' })
    ).toBeInTheDocument()
  })
})

describe('ShareButton — variants and labelling', () => {
  it('renders the bracket variant for dense entity headers', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })

    render(
      <ShareButton
        path="/artists/foo"
        variant="bracket"
        ariaLabel="Share this artist"
      />
    )
    const button = await screen.findByRole('button', {
      name: 'Share this artist',
    })
    expect(button).toHaveTextContent('[Share]')
  })

  it('uses the caller-supplied accessible name', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setCapabilities({ writeText })

    render(<ShareButton path={SHOW_PATH} ariaLabel="Share this show" />)
    expect(
      await screen.findByRole('button', { name: 'Share this show' })
    ).toBeInTheDocument()
  })
})
