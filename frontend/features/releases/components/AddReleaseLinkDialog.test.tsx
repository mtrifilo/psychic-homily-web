import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
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

  it('offers all 7 external link platforms in the Select', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('combobox', { name: 'Link platform' }))

    // Radix renders options into a portaled listbox.
    const listbox = await screen.findByRole('listbox')
    const options = within(listbox).getAllByRole('option')
    expect(options).toHaveLength(7)
    expect(within(listbox).getByText('Bandcamp')).toBeInTheDocument()
    expect(within(listbox).getByText('Spotify')).toBeInTheDocument()
    expect(within(listbox).getByText('Apple Music')).toBeInTheDocument()
    expect(within(listbox).getByText('YouTube')).toBeInTheDocument()
    expect(within(listbox).getByText('Discogs')).toBeInTheDocument()
    expect(within(listbox).getByText('Tidal')).toBeInTheDocument()
    expect(within(listbox).getByText('SoundCloud')).toBeInTheDocument()
  })

  it('shows a validation error and blocks submit for a malformed URL', async () => {
    const user = userEvent.setup()
    renderDialog()

    const urlInput = screen.getByLabelText('URL')
    await user.type(urlInput, 'not-a-url')

    expect(
      screen.getByText(/Enter a valid URL starting with http/i)
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
