import { describe, expect, it, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { AuthErrorCode, isAuthError } from '@/lib/errors'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      OAUTH_PENDING_LINK: '/auth/oauth/pending-link',
      OAUTH_LINK_BY_EMAIL: '/auth/oauth/link-by-email',
    },
  },
}))

import { usePendingOAuthLink, useSendOAuthLinkEmail } from './useOAuthLink'

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('usePendingOAuthLink', () => {
  beforeEach(() => mockApiRequest.mockReset())

  it('reads the pending link when enabled', async () => {
    mockApiRequest.mockResolvedValue({ status: 'pending', email: 'alice@example.com' })

    const { result } = renderHook(() => usePendingOAuthLink(true), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual({ status: 'pending', email: 'alice@example.com' })
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/oauth/pending-link',
      expect.objectContaining({ method: 'GET' })
    )
  })

  it('reads nothing when disabled', () => {
    renderHook(() => usePendingOAuthLink(false), { wrapper })
    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})

describe('useSendOAuthLinkEmail', () => {
  beforeEach(() => mockApiRequest.mockReset())

  it('POSTs with no body; the cookie names the pending link', async () => {
    mockApiRequest.mockResolvedValue({ success: true, message: 'Check your inbox' })

    const { result } = renderHook(() => useSendOAuthLinkEmail(), { wrapper })
    result.current.mutate()

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/auth/oauth/link-by-email', { method: 'POST' })
  })

  it('throws a refusal as an AuthError carrying its code', async () => {
    mockApiRequest.mockResolvedValue({
      success: false,
      message: 'gone',
      error_code: 'OAUTH_LINK_PENDING_EXPIRED',
    })

    const { result } = renderHook(() => useSendOAuthLinkEmail(), { wrapper })
    result.current.mutate()

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(isAuthError(result.current.error)).toBe(true)
    expect(isAuthError(result.current.error) && result.current.error.code).toBe(
      AuthErrorCode.OAUTH_LINK_PENDING_EXPIRED
    )
  })
})
