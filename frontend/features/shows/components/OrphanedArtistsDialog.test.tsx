import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { OrphanedArtistsDialog } from './OrphanedArtistsDialog'
import type { OrphanedArtist } from '../types'

// --- Mocks ---

// The dialog imports both `apiRequest` and `API_ENDPOINTS` from `@/lib/api`.
// Mock that surface so deletes hit a spy and the endpoint builder is stable.
const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ARTISTS: {
      DELETE: (id: string | number) => `/artists/${id}`,
    },
  },
}))

// --- Helpers ---

function makeArtist(overrides: Partial<OrphanedArtist> = {}): OrphanedArtist {
  return {
    id: 1,
    name: 'Orphan A',
    slug: 'orphan-a',
    ...overrides,
  }
}

describe('OrphanedArtistsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('rendering', () => {
    it('renders the orphaned artist list', () => {
      const artists = [
        makeArtist({ id: 1, name: 'Orphan A' }),
        makeArtist({ id: 2, name: 'Orphan B' }),
      ]
      renderWithProviders(
        <OrphanedArtistsDialog
          open
          onOpenChange={vi.fn()}
          artists={artists}
        />
      )

      expect(screen.getByText('Orphan A')).toBeInTheDocument()
      expect(screen.getByText('Orphan B')).toBeInTheDocument()
    })

    it('pluralizes the delete button label by artist count', () => {
      const one = [makeArtist({ id: 1 })]
      const { rerender } = renderWithProviders(
        <OrphanedArtistsDialog open onOpenChange={vi.fn()} artists={one} />
      )
      expect(
        screen.getByRole('button', { name: /Delete Artist$/i })
      ).toBeInTheDocument()

      rerender(
        <OrphanedArtistsDialog
          open
          onOpenChange={vi.fn()}
          artists={[makeArtist({ id: 1 }), makeArtist({ id: 2 })]}
        />
      )
      expect(
        screen.getByRole('button', { name: /Delete Artists$/i })
      ).toBeInTheDocument()
    })

    it('does not render dialog content when closed', () => {
      renderWithProviders(
        <OrphanedArtistsDialog
          open={false}
          onOpenChange={vi.fn()}
          artists={[makeArtist()]}
        />
      )
      expect(screen.queryByText('Orphan A')).not.toBeInTheDocument()
    })
  })

  describe('keep all', () => {
    it('closes the dialog and fires onComplete without deleting', async () => {
      const user = userEvent.setup()
      const onOpenChange = vi.fn()
      const onComplete = vi.fn()
      renderWithProviders(
        <OrphanedArtistsDialog
          open
          onOpenChange={onOpenChange}
          artists={[makeArtist()]}
          onComplete={onComplete}
        />
      )

      await user.click(screen.getByRole('button', { name: /Keep All/i }))

      expect(onOpenChange).toHaveBeenCalledWith(false)
      expect(onComplete).toHaveBeenCalledTimes(1)
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('delete', () => {
    it('issues a DELETE per artist, then closes and fires onComplete', async () => {
      const user = userEvent.setup()
      const onOpenChange = vi.fn()
      const onComplete = vi.fn()
      mockApiRequest.mockResolvedValue(undefined)

      const artists = [
        makeArtist({ id: 1, name: 'Orphan A' }),
        makeArtist({ id: 2, name: 'Orphan B' }),
      ]
      renderWithProviders(
        <OrphanedArtistsDialog
          open
          onOpenChange={onOpenChange}
          artists={artists}
          onComplete={onComplete}
        />
      )

      await user.click(screen.getByRole('button', { name: /Delete Artists/i }))

      await waitFor(() =>
        expect(onComplete).toHaveBeenCalledTimes(1)
      )
      expect(mockApiRequest).toHaveBeenCalledTimes(2)
      expect(mockApiRequest).toHaveBeenCalledWith('/artists/1', {
        method: 'DELETE',
      })
      expect(mockApiRequest).toHaveBeenCalledWith('/artists/2', {
        method: 'DELETE',
      })
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })

    it('shows an error and stays open when a delete fails', async () => {
      const user = userEvent.setup()
      const onOpenChange = vi.fn()
      const onComplete = vi.fn()
      mockApiRequest.mockRejectedValue(new Error('still associated'))

      renderWithProviders(
        <OrphanedArtistsDialog
          open
          onOpenChange={onOpenChange}
          artists={[makeArtist()]}
          onComplete={onComplete}
        />
      )

      await user.click(screen.getByRole('button', { name: /Delete Artist/i }))

      expect(
        await screen.findByText(/Failed to delete some artists/i)
      ).toBeInTheDocument()
      // On failure the dialog must not close or signal completion.
      expect(onOpenChange).not.toHaveBeenCalled()
      expect(onComplete).not.toHaveBeenCalled()
    })

    it('disables both buttons while a delete is in flight', async () => {
      const user = userEvent.setup()
      // Hold the request open so the pending UI is observable.
      let resolveRequest: () => void = () => {}
      mockApiRequest.mockImplementation(
        () =>
          new Promise<void>(resolve => {
            resolveRequest = resolve
          })
      )

      renderWithProviders(
        <OrphanedArtistsDialog
          open
          onOpenChange={vi.fn()}
          artists={[makeArtist()]}
        />
      )

      await user.click(screen.getByRole('button', { name: /Delete Artist/i }))

      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: /Deleting/i })
        ).toBeDisabled()
      )
      expect(screen.getByRole('button', { name: /Keep All/i })).toBeDisabled()

      resolveRequest()
    })
  })
})
