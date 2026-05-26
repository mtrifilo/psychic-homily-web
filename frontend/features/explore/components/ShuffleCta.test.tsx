import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ShuffleCta } from './ShuffleCta'
import type { ExploreShuffleTargetResponse } from '../types'

const mockRouterPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockRouterPush }),
}))

type RefetchResult = { data: ExploreShuffleTargetResponse | undefined }
const mockRefetch = vi.fn<() => Promise<RefetchResult>>()
const mockHookState = {
  isFetching: false,
}
vi.mock('../hooks', () => ({
  useShuffleTarget: () => ({
    refetch: mockRefetch,
    isFetching: mockHookState.isFetching,
  }),
}))

beforeEach(() => {
  mockRouterPush.mockReset()
  mockRefetch.mockReset()
  mockHookState.isFetching = false
})

describe('ShuffleCta', () => {
  it('renders the button label', () => {
    render(<ShuffleCta />)
    expect(
      screen.getByRole('button', { name: /drop me somewhere/i }),
    ).toBeInTheDocument()
  })

  it('navigates to the resolved artist page on click', async () => {
    mockRefetch.mockResolvedValue({
      data: {
        artist_id: 7,
        artist_slug: 'cool-band',
        artist_name: 'Cool Band',
      },
    })
    render(<ShuffleCta />)

    fireEvent.click(screen.getByRole('button', { name: /drop me somewhere/i }))

    await waitFor(() =>
      expect(mockRouterPush).toHaveBeenCalledWith('/artists/cool-band'),
    )
  })

  it('shows an inline message when no shuffle target is returned', async () => {
    mockRefetch.mockResolvedValue({
      data: {
        artist_id: null,
        artist_slug: null,
        artist_name: null,
      },
    })
    render(<ShuffleCta />)

    fireEvent.click(screen.getByRole('button', { name: /drop me somewhere/i }))

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(/no artists/i),
    )
    expect(mockRouterPush).not.toHaveBeenCalled()
  })

  it('surfaces an error message when the refetch throws', async () => {
    mockRefetch.mockRejectedValue(new Error('boom'))
    render(<ShuffleCta />)

    fireEvent.click(screen.getByRole('button', { name: /drop me somewhere/i }))

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(/could not pick/i),
    )
  })
})
