import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import type { EditableEntityType } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
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
})
