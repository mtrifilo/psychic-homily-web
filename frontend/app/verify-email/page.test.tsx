import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, waitFor } from '@/test/utils'
import VerifyEmailPage from './page'

const mockMutate = vi.fn()
const mockUseConfirmVerification = vi.fn()

vi.mock('next/navigation', () => ({
  useSearchParams: () => new URLSearchParams('token=verify-token'),
}))

vi.mock('@/lib/hooks/useAuth', () => ({
  useConfirmVerification: () => mockUseConfirmVerification(),
}))

describe('VerifyEmailPage', () => {
  beforeEach(() => {
    mockMutate.mockReset()
    mockUseConfirmVerification.mockReset()

    mockUseConfirmVerification.mockImplementation(() => ({
      mutate: mockMutate,
      isPending: false,
      isError: false,
      isSuccess: false,
      error: null,
    }))
  })

  it('verifies email only once per token across re-renders', async () => {
    const { rerender } = renderWithProviders(<VerifyEmailPage />)

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledTimes(1)
      expect(mockMutate).toHaveBeenCalledWith('verify-token')
    })

    rerender(<VerifyEmailPage />)

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledTimes(1)
    })
  })
})
