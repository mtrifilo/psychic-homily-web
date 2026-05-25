import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateAdminEntityReports = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      ENTITY_REPORTS: {
        LIST: '/admin/entity-reports',
        RESOLVE: (reportId: string | number) =>
          `/admin/entity-reports/${reportId}/resolve`,
        DISMISS: (reportId: string | number) =>
          `/admin/entity-reports/${reportId}/dismiss`,
      },
    },
    COLLECTIONS: {
      DETAIL: (slug: string) => `/collections/${slug}`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      entityReports: (params: Record<string, unknown>) =>
        ['admin', 'entityReports', params],
    },
  },
  createInvalidateQueries: () => ({
    adminEntityReports: mockInvalidateAdminEntityReports,
  }),
}))

import {
  useAdminEntityReports,
  useResolveEntityReport,
  useDismissEntityReport,
  useAdminHideCollection,
} from './useAdminEntityReports'

describe('useAdminEntityReports', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches reports with default filters (status=pending)', async () => {
    const mockResponse = {
      reports: [
        {
          id: 1,
          entity_type: 'collection',
          entity_id: 42,
          reported_by: 7,
          reporter_username: 'alice',
          report_type: 'spam',
          status: 'pending',
          created_at: '2026-04-01T00:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminEntityReports(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/entity-reports')
    expect(url).toContain('status=pending')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
    expect(url).not.toContain('entity_type=')
    expect(result.current.data?.reports).toHaveLength(1)
  })

  it('passes entity_type filter when provided', async () => {
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(
      () =>
        useAdminEntityReports({
          entity_type: 'collection',
          limit: 25,
          offset: 50,
        }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('entity_type=collection')
    expect(url).toContain('limit=25')
    expect(url).toContain('offset=50')
  })

  it('omits status param when status is explicitly empty', async () => {
    // When called with status='' the hook should skip the param so the
    // backend default surface (all statuses) is used.
    mockApiRequest.mockResolvedValueOnce({ reports: [], total: 0 })

    const { result } = renderHook(
      () => useAdminEntityReports({ status: '' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain('status=')
  })

  it('surfaces fetch errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdminEntityReports(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Forbidden')
  })
})

describe('useResolveEntityReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAdminEntityReports.mockReset()
  })

  it('POSTs to the resolve endpoint with notes', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, status: 'resolved' })

    const { result } = renderHook(() => useResolveEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1, notes: 'Reviewed' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/entity-reports/1/resolve',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ notes: 'Reviewed' }),
      })
    )
  })

  it('sends empty-string notes when omitted', async () => {
    // The hook coerces undefined notes to '' so the backend never sees
    // a missing field — this guards that contract.
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useResolveEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/entity-reports/1/resolve',
      expect.objectContaining({
        body: JSON.stringify({ notes: '' }),
      })
    )
  })

  it('invalidates admin entity reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1 })

    const { result } = renderHook(() => useResolveEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 1 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateAdminEntityReports).toHaveBeenCalledTimes(1)
  })

  it('surfaces mutation errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Report not found'))

    const { result } = renderHook(() => useResolveEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 999 })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Report not found')
    expect(mockInvalidateAdminEntityReports).not.toHaveBeenCalled()
  })
})

describe('useDismissEntityReport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAdminEntityReports.mockReset()
  })

  it('POSTs to the dismiss endpoint with notes', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 2, status: 'dismissed' })

    const { result } = renderHook(() => useDismissEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 2, notes: 'Not actionable' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/entity-reports/2/dismiss',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ notes: 'Not actionable' }),
      })
    )
  })

  it('sends empty-string notes when omitted', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 2 })

    const { result } = renderHook(() => useDismissEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 2 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/entity-reports/2/dismiss',
      expect.objectContaining({
        body: JSON.stringify({ notes: '' }),
      })
    )
  })

  it('invalidates admin entity reports on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 2 })

    const { result } = renderHook(() => useDismissEntityReport(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ reportId: 2 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateAdminEntityReports).toHaveBeenCalledTimes(1)
  })
})

describe('useAdminHideCollection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAdminEntityReports.mockReset()
  })

  it('PUTs is_public=false to the collection detail endpoint', async () => {
    // This hook re-uses the existing admin-permitted PUT /collections/{slug}
    // rather than introducing a new endpoint. The body shape is the small
    // {is_public: false} patch the moderation queue depends on.
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useAdminHideCollection(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ slug: 'spammy-list' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/collections/spammy-list',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ is_public: false }),
      })
    )
  })

  it('invalidates admin entity reports AND collections on success', async () => {
    // The moderation queue surface depends on both invalidations: the
    // queue itself (so the related report disappears) AND the collections
    // list (so the now-private record stops appearing in browse).
    const { QueryClient, QueryClientProvider } = await import(
      '@tanstack/react-query'
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    mockApiRequest.mockResolvedValueOnce(undefined)

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useAdminHideCollection(), { wrapper })

    await act(async () => {
      result.current.mutate({ slug: 'spammy-list' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockInvalidateAdminEntityReports).toHaveBeenCalledTimes(1)
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['collections'],
    })
  })
})
