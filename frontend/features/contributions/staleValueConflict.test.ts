import { describe, it, expect } from 'vitest'
import { staleFieldCurrentValue, STALE_VALUE_CODE } from './staleValueConflict'
import type { ApiError } from '@/lib/api'

/**
 * Builds the shape apiRequest produces for a huma error body: `details` is the
 * `errors` array, and each entry's `value` carries the code and the per-field
 * current values (huma's error model has no field for a code).
 */
function conflict(value: unknown): ApiError {
  const error: ApiError = new Error('This field has changed')
  error.status = 409
  error.details = [{ message: 'This field has changed', location: 'body.changes', value }]
  return error
}

describe('staleFieldCurrentValue', () => {
  it('reads the named field from a stale-value 409', () => {
    const error = conflict({
      code: STALE_VALUE_CODE,
      current_values: { description: 'Mesa emo, formed 1993.' },
    })
    expect(staleFieldCurrentValue(error, 'description')).toBe('Mesa emo, formed 1993.')
  })

  it('reads an empty current value as the value it is, not as absence', () => {
    const error = conflict({ code: STALE_VALUE_CODE, current_values: { description: '' } })
    expect(staleFieldCurrentValue(error, 'description')).toBe('')
  })

  it('reports nothing for a field the refusal did not name', () => {
    const error = conflict({ code: STALE_VALUE_CODE, current_values: { name: 'Other' } })
    expect(staleFieldCurrentValue(error, 'description')).toBeUndefined()
  })

  // A duplicate queued edit is also a 409 and carries no values, so the status
  // alone cannot decide this.
  it('reports nothing for a 409 that is not a stale-value conflict', () => {
    const duplicate: ApiError = new Error('you already have a pending edit for this entity')
    duplicate.status = 409
    duplicate.details = { detail: 'you already have a pending edit for this entity' }
    expect(staleFieldCurrentValue(duplicate, 'description')).toBeUndefined()
  })

  it('reports nothing for a detail carrying a different code', () => {
    const error = conflict({
      code: 'COLLECTION_LIMIT',
      current_values: { description: 'not this one' },
    })
    expect(staleFieldCurrentValue(error, 'description')).toBeUndefined()
  })

  it('reports nothing for a non-string current value', () => {
    const error = conflict({ code: STALE_VALUE_CODE, current_values: { capacity: 550 } })
    expect(staleFieldCurrentValue(error, 'capacity')).toBeUndefined()
  })

  it('reports nothing for a non-409 error, a plain error, or a non-object', () => {
    const notFound: ApiError = new Error('gone')
    notFound.status = 404
    notFound.details = [{ value: { code: STALE_VALUE_CODE, current_values: { description: 'x' } } }]
    expect(staleFieldCurrentValue(notFound, 'description')).toBeUndefined()
    expect(staleFieldCurrentValue(new Error('boom'), 'description')).toBeUndefined()
    expect(staleFieldCurrentValue(null, 'description')).toBeUndefined()
    expect(staleFieldCurrentValue('nope', 'description')).toBeUndefined()
  })
})
