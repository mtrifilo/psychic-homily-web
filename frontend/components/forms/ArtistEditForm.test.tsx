import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { ArtistEditForm } from './ArtistEditForm'
import type { Artist } from '@/features/artists'

// --- Mocks ---

// `useArtistUpdate` is imported directly from the admin-artists hook module.
// A single shared mutate spy lets each test assert what was sent and drive the
// success / error callbacks the form wires up.
const mockMutate = vi.fn()
let mockIsPending = false

vi.mock('@/lib/hooks/admin/useAdminArtists', () => ({
  useArtistUpdate: () => ({ mutate: mockMutate, isPending: mockIsPending }),
}))

// --- Helpers ---

function makeArtist(overrides: Partial<Artist> = {}): Artist {
  return {
    id: 1,
    slug: 'artist-a',
    name: 'Artist A',
    city: 'Phoenix',
    state: 'AZ',
    bandcamp_embed_url: null,
    social: {
      instagram: 'https://instagram.com/artist-a',
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: 'https://artist-a.com',
    },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ArtistEditForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockIsPending = false
  })

  describe('initial render', () => {
    it('populates fields from the artist prop', () => {
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm
          key={artist.id}
          artist={artist}
          open
          onOpenChange={vi.fn()}
        />
      )

      expect(screen.getByLabelText(/Name \*/i)).toHaveValue('Artist A')
      expect(screen.getByLabelText(/City/i)).toHaveValue('Phoenix')
      expect(screen.getByLabelText(/State/i)).toHaveValue('AZ')
      expect(screen.getByLabelText(/Instagram/i)).toHaveValue(
        'https://instagram.com/artist-a'
      )
      expect(screen.getByLabelText(/Website/i)).toHaveValue(
        'https://artist-a.com'
      )
    })

    it('renders empty strings for absent optional fields', () => {
      const artist = makeArtist({
        city: null,
        state: null,
        social: {
          instagram: null,
          facebook: null,
          twitter: null,
          youtube: null,
          spotify: null,
          soundcloud: null,
          bandcamp: null,
          website: null,
        },
      })
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      expect(screen.getByLabelText(/City/i)).toHaveValue('')
      expect(screen.getByLabelText(/Spotify/i)).toHaveValue('')
      // Name is always present.
      expect(screen.getByLabelText(/Name \*/i)).toHaveValue('Artist A')
    })
  })

  describe('field edits + submit', () => {
    it('calls the update mutation with only the changed fields', async () => {
      const user = userEvent.setup()
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      const nameInput = screen.getByLabelText(/Name \*/i)
      await user.clear(nameInput)
      await user.type(nameInput, 'Artist B')

      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      await waitFor(() => expect(mockMutate).toHaveBeenCalledTimes(1))
      // Only `name` changed — city/state/socials must be omitted from the diff.
      expect(mockMutate).toHaveBeenCalledWith(
        { artistId: 1, data: { name: 'Artist B' } },
        expect.objectContaining({
          onSuccess: expect.any(Function),
          onError: expect.any(Function),
        })
      )
    })

    it('does not call the mutation and shows "No changes detected" on an unchanged submit', async () => {
      const user = userEvent.setup()
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(await screen.findByText(/No changes detected/i)).toBeInTheDocument()
      expect(mockMutate).not.toHaveBeenCalled()
    })

    it('surfaces a validation message and blocks the mutation when the required name is cleared', async () => {
      // The zod `onSubmit` validator rejects an empty name. The form must both
      // block the mutation AND surface the message inline (via FieldInfo), so
      // the user isn't left with a silent, dead Save button.
      const user = userEvent.setup()
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      await user.clear(screen.getByLabelText(/Name \*/i))
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(
        await screen.findByText(/Artist name is required/i)
      ).toBeInTheDocument()
      expect(mockMutate).not.toHaveBeenCalled()
    })
  })

  describe('mutation result handling', () => {
    it('shows an error alert when the mutation fails', async () => {
      const user = userEvent.setup()
      // Drive the onError callback the form passes to mutate().
      mockMutate.mockImplementation((_vars, { onError }) => {
        onError(new Error('Server exploded'))
      })

      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      const nameInput = screen.getByLabelText(/Name \*/i)
      await user.clear(nameInput)
      await user.type(nameInput, 'Artist B')
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(await screen.findByText(/Server exploded/i)).toBeInTheDocument()
    })

    it('shows the success alert, then closes the dialog and fires onSuccess', async () => {
      const user = userEvent.setup()
      const onOpenChange = vi.fn()
      const onSuccess = vi.fn()
      // Drive the onSuccess callback the form passes to mutate().
      mockMutate.mockImplementation((_vars, { onSuccess: cbSuccess }) => {
        cbSuccess()
      })

      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm
          artist={artist}
          open
          onOpenChange={onOpenChange}
          onSuccess={onSuccess}
        />
      )

      const nameInput = screen.getByLabelText(/Name \*/i)
      await user.clear(nameInput)
      await user.type(nameInput, 'Artist B')
      await user.click(screen.getByRole('button', { name: /Save Changes/i }))

      expect(
        await screen.findByText(/Artist updated successfully/i)
      ).toBeInTheDocument()

      // Close + onSuccess are deferred behind a real 1500ms timer; wait it out
      // with real timers (fake timers + userEvent + findByText interact
      // badly enough to hang sibling tests when not perfectly torn down).
      await waitFor(
        () => {
          expect(onOpenChange).toHaveBeenCalledWith(false)
          expect(onSuccess).toHaveBeenCalledTimes(1)
        },
        { timeout: 2500 }
      )
    })
  })

  describe('cancel', () => {
    it('calls onOpenChange(false) when Cancel is clicked', async () => {
      const user = userEvent.setup()
      const onOpenChange = vi.fn()
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={onOpenChange} />
      )

      await user.click(screen.getByRole('button', { name: /Cancel/i }))

      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  describe('pending state', () => {
    it('disables Save and shows the saving label while the mutation is pending', () => {
      mockIsPending = true
      const artist = makeArtist()
      renderWithProviders(
        <ArtistEditForm artist={artist} open onOpenChange={vi.fn()} />
      )

      const saveButton = screen.getByRole('button', { name: /Saving/i })
      expect(saveButton).toBeDisabled()
      expect(screen.getByRole('button', { name: /Cancel/i })).toBeDisabled()
    })
  })

  describe('artist switch resets fields via key prop', () => {
    it('resets fields when re-rendered with a different artist (via key prop)', async () => {
      const user = userEvent.setup()
      const artistA = makeArtist({
        id: 1,
        name: 'Artist A',
        city: 'Phoenix',
        state: 'AZ',
      })
      const artistB = makeArtist({
        id: 2,
        name: 'Artist B',
        city: 'Tucson',
        state: 'AZ',
        social: {
          instagram: 'https://instagram.com/artist-b',
          facebook: null,
          twitter: null,
          youtube: null,
          spotify: null,
          soundcloud: null,
          bandcamp: null,
          website: 'https://artist-b.com',
        },
      })

      const { rerender } = renderWithProviders(
        <ArtistEditForm
          key={artistA.id}
          artist={artistA}
          open
          onOpenChange={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText(/Name \*/i)
      expect(nameInput).toHaveValue('Artist A')

      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')
      expect(nameInput).toHaveValue('Dirty Edit')

      rerender(
        <ArtistEditForm
          key={artistB.id}
          artist={artistB}
          open
          onOpenChange={vi.fn()}
        />
      )

      // Re-query after rerender — the key change unmounts the previous
      // input node, so `nameInput` no longer points at a live element.
      expect(screen.getByLabelText(/Name \*/i)).toHaveValue('Artist B')
      expect(screen.getByLabelText(/City/i)).toHaveValue('Tucson')
      expect(screen.getByLabelText(/Website/i)).toHaveValue(
        'https://artist-b.com'
      )
    })

    it('preserves dirty edits when re-rendered with the same key', async () => {
      // Pins the `key` as the load-bearing reset mechanism: if React
      // re-renders the same instance (no key change), the dirty edit
      // must survive. Without this, a future maintainer could
      // accidentally add an artist-prop-based reset and have both tests
      // still pass.
      const user = userEvent.setup()
      const artist = makeArtist({ id: 1, name: 'Artist A' })

      const { rerender } = renderWithProviders(
        <ArtistEditForm
          key={artist.id}
          artist={artist}
          open
          onOpenChange={vi.fn()}
        />
      )

      const nameInput = screen.getByLabelText(/Name \*/i)
      await user.clear(nameInput)
      await user.type(nameInput, 'Dirty Edit')

      rerender(
        <ArtistEditForm
          key={artist.id}
          artist={artist}
          open
          onOpenChange={vi.fn()}
        />
      )

      expect(screen.getByLabelText(/Name \*/i)).toHaveValue('Dirty Edit')
    })
  })
})
