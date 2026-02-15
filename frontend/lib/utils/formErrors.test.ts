import { describe, it, expect } from 'vitest'
import { getErrorMessage, getUniqueErrors } from './formErrors'

describe('getErrorMessage', () => {
  it('returns string errors as-is', () => {
    expect(getErrorMessage('field required')).toBe('field required')
  })

  it('extracts message property from error objects', () => {
    expect(getErrorMessage({ message: 'invalid email' })).toBe('invalid email')
  })

  it('converts non-string message to string', () => {
    expect(getErrorMessage({ message: 42 })).toBe('42')
  })

  it('stringifies unknown types', () => {
    expect(getErrorMessage(123)).toBe('123')
  })

  it('handles null', () => {
    expect(getErrorMessage(null)).toBe('null')
  })
})

describe('getUniqueErrors', () => {
  it('joins multiple unique errors with commas', () => {
    expect(getUniqueErrors(['error A', 'error B'])).toBe('error A, error B')
  })

  it('deduplicates identical error strings', () => {
    expect(getUniqueErrors(['same', 'same', 'same'])).toBe('same')
  })

  it('deduplicates mixed string and object errors with same message', () => {
    expect(getUniqueErrors(['invalid', { message: 'invalid' }])).toBe('invalid')
  })

  it('returns empty string for empty array', () => {
    expect(getUniqueErrors([])).toBe('')
  })
})
