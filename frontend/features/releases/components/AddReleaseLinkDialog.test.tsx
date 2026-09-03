import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import {
  OFFERED_RELEASE_LINK_PLATFORM_KEYS,
  RELEASE_LINK_PLATFORMS,
  RELEASE_LINK_PLATFORM_KEYS,
} from '@/lib/releaseLinks'
import { AddReleaseLinkDialog } from './AddReleaseLinkDialog'

// Capture the add-link mutation so flows can assert wiring without a backend.
const mockMutate = vi.fn()
const mockReset = vi.fn()
let mutationState = {
  mutate: mockMutate,
  reset: mockReset,
  isPending: false,
  isSuccess: false,
  isError: false,
  error: null as Error | null,
}
vi.mock('../hooks/useAdminReleases', () => ({
  useAddReleaseLink: () => mutationState,
}))

function renderDialog() {
  return renderWithProviders(
    <AddReleaseLinkDialog
      open
      onOpenChange={vi.fn()}
      releaseId={42}
      releaseTitle="In Rainbows"
    />
  )
}

describe('AddReleaseLinkDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mutationState = {
      mutate: mockMutate,
      reset: mockReset,
      isPending: false,
      isSuccess: false,
      isError: false,
      error: null,
    }
  })

  it('renders the dialog header quoting the release title', () => {
    renderDialog()
    const dialog = screen.getByRole('dialog')
    // "Add link" also labels the submit button, so target the title heading.
    expect(
      within(dialog).getByRole('heading', { name: /Add link/ })
    ).toBeInTheDocument()
    expect(within(dialog).getByText(/In Rainbows/)).toBeInTheDocument()
  })

  // Driven by the registry rather than a hand-copied count: the picker, the
  // render gate and the backend write gate read one list, so a platform marked
  // offered has to appear here without editing this test.
  it('offers every offered platform in the Select, and only those', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('combobox', { name: 'Link platform' }))

    // Radix renders options into a portaled listbox.
    const listbox = await screen.findByRole('listbox')
    const options = within(listbox).getAllByRole('option')
    expect(options).toHaveLength(OFFERED_RELEASE_LINK_PLATFORM_KEYS.length)
    for (const key of RELEASE_LINK_PLATFORM_KEYS) {
      const label = RELEASE_LINK_PLATFORMS[key].label
      if (RELEASE_LINK_PLATFORMS[key].offered) {
        expect(within(listbox).getByText(label)).toBeInTheDocument()
      } else {
        expect(within(listbox).queryByText(label)).not.toBeInTheDocument()
      }
    }
  })

  // The host anchor is enforced before the round trip, using the same predicate
  // the backend and the release page run.
  it('blocks submit and names the accepted host for an off-platform URL', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.type(
      screen.getByLabelText('URL'),
      'https://bandcamp-checkout.evil.test/album/x'
    )

    expect(await screen.findByText(/must be an http or https URL on bandcamp\.com/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Add link/ })).toBeDisabled()
  })

  it('accepts a URL on the selected platform', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.type(
      screen.getByLabelText('URL'),
      'https://kingbuffalo.bandcamp.com/album/regenerator'
    )

    expect(screen.queryByText(/must be an http or https URL/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Add link/ })).toBeEnabled()
  })

  it('shows a validation error and blocks submit for a malformed URL', async () => {
    const user = userEvent.setup()
    renderDialog()

    const urlInput = screen.getByLabelText('URL')
    await user.type(urlInput, 'not-a-url')

    // One refusal covers every way the value can be wrong, and names the shape
    // that works instead of the check that failed.
    expect(
      screen.getByText(/must be an http or https URL on bandcamp\.com/)
    ).toBeInTheDocument()

    const submit = screen.getByRole('button', { name: /Add link/i })
    expect(submit).toBeDisabled()
    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('keeps submit disabled while the URL is empty', () => {
    renderDialog()
    const submit = screen.getByRole('button', { name: /Add link/i })
    expect(submit).toBeDisabled()
  })

  it('submits the platform + URL for a valid entry', async () => {
    const user = userEvent.setup()
    renderDialog()

    const urlInput = screen.getByLabelText('URL')
    await user.type(urlInput, 'https://radiohead.bandcamp.com/album/in-rainbows')

    const submit = screen.getByRole('button', { name: /Add link/i })
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    expect(mockMutate).toHaveBeenCalledTimes(1)
    const [payload] = mockMutate.mock.calls[0]
    expect(payload).toMatchObject({
      releaseId: 42,
      platform: 'bandcamp',
      url: 'https://radiohead.bandcamp.com/album/in-rainbows',
    })
  })

  it('renders a success banner and Close button after a successful add', async () => {
    const user = userEvent.setup()
    // Drive the success path: mutate fires its onSuccess callback (the real
    // hook does this after the POST resolves), and the mutation reports success.
    mockMutate.mockImplementation((_payload, opts) => {
      mutationState = { ...mutationState, isSuccess: true }
      opts?.onSuccess?.()
    })
    mutationState = { ...mutationState, isSuccess: true }
    renderDialog()

    const urlInput = screen.getByLabelText('URL')
    await user.type(urlInput, 'https://radiohead.bandcamp.com/album/x')
    const submit = screen.getByRole('button', { name: /Add link/i })
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    // Form is replaced by the success banner; the footer Close button appears.
    // (Radix Dialog also renders a built-in close "X" with an sr-only "Close"
    // label, so there are two "Close" buttons — assert the footer one exists.)
    expect(screen.getByText('Link added')).toBeInTheDocument()
    expect(screen.queryByLabelText('URL')).not.toBeInTheDocument()
    const closeButtons = screen.getAllByRole('button', { name: 'Close' })
    const footerClose = closeButtons.find(
      (b) => b.getAttribute('data-slot') === 'button'
    )
    expect(footerClose).toBeDefined()
  })

  it('shows the backend error message when the mutation fails', () => {
    mutationState = {
      ...mutationState,
      isError: true,
      error: new Error('You do not have permission to add release links'),
    }
    renderDialog()
    expect(
      screen.getByText('You do not have permission to add release links')
    ).toBeInTheDocument()
  })
})
