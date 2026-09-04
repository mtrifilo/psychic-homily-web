import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'
import type { EditableEntityType } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
  isConflictError: (error: unknown) =>
    error !== null && typeof error === 'object' && (error as { status?: number }).status === 409,
}))

// Import after mocks are wired.
import { useSuggestEdit } from './useSuggestEdit'

describe('useSuggestEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // Enumerating every EditableEntityType is the verification that the
  // ENTITY_PLURAL map stays in sync with the union — if a new entity is
  // added without a matching plural, TS catches the map and this test
  // catches the URL shape.
  const cases: Array<{ entityType: EditableEntityType; expectedPlural: string }> = [
    { entityType: 'artist', expectedPlural: 'artists' },
    { entityType: 'venue', expectedPlural: 'venues' },
    { entityType: 'festival', expectedPlural: 'festivals' },
    { entityType: 'release', expectedPlural: 'releases' },
    { entityType: 'label', expectedPlural: 'labels' },
    { entityType: 'show', expectedPlural: 'shows' },
  ]

  it.each(cases)(
    'builds the suggest-edit URL for $entityType using the plural map',
    async ({ entityType, expectedPlural }) => {
      mockApiRequest.mockResolvedValueOnce({
        applied: true,
        message: 'ok',
      })

      const { result } = renderHook(() => useSuggestEdit(), {
        wrapper: createWrapper(),
      })

      result.current.mutate({
        entityType,
        entityId: 42,
        changes: [{ field: 'description', old_value: 'old', new_value: 'new' }],
        summary: 'tighten copy',
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        `http://localhost:8080/${expectedPlural}/42/suggest-edit`,
        {
          method: 'PUT',
          body: JSON.stringify({
            changes: [{ field: 'description', old_value: 'old', new_value: 'new' }],
            summary: 'tighten copy',
          }),
        }
      )
    }
  )

  // A 409 says the entity is not in the state the submission described, so the
  // cached entity the form reads its previous values from is known stale. If it
  // is not refetched, the form keeps asserting the value the server just
  // rejected and every resubmission fails the same way.
  it('refetches the entity when the server answers 409', async () => {
    const conflict = Object.assign(
      new Error('This field has changed since you loaded the form: name.'),
      { status: 409 }
    )
    mockApiRequest.mockRejectedValueOnce(conflict)

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useSuggestEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate({
      entityType: 'artist',
      entityId: 42,
      changes: [{ field: 'name', old_value: 'Stale', new_value: 'New' }],
      summary: 'rename',
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['artists'] })
  })

  // Every other failure leaves the cached entity alone: a 422 or a 500 says
  // nothing about whether the entity moved.
  it('leaves the cached entity alone on a non-conflict failure', async () => {
    mockApiRequest.mockRejectedValueOnce(
      Object.assign(new Error('Summary is required'), { status: 422 })
    )

    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const { result } = renderHook(() => useSuggestEdit(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate({
      entityType: 'artist',
      entityId: 42,
      changes: [{ field: 'name', old_value: 'Old', new_value: 'New' }],
      summary: '',
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})
