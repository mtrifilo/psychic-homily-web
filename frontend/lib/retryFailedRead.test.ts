import { describe, it, expect } from 'vitest'
import { AuthError } from '@/lib/errors'
import { RETRY_FAILED_READ_MS, retryWhileFailed } from './retryFailedRead'

describe('retryWhileFailed', () => {
  it.each([
    ['a failed read', { status: 'error', error: new Error('502') }, RETRY_FAILED_READ_MS],
    [
      'a refused session',
      { status: 'error', error: new AuthError('x', 'UNAUTHORIZED' as never, { status: 401 }) },
      false,
    ],
    ['a successful read', { status: 'success', error: null }, false],
    ['a read in flight', { status: 'pending', error: null }, false],
  ] as const)('%s', (_label, state, want) => {
    expect(retryWhileFailed({ state })).toBe(want)
  })
})
