import { describe, it, expect } from 'vitest'
import { isAbortError, isTimeoutError } from './abortErrors'

describe('isAbortError', () => {
  it.each([
    ['a DOMException named AbortError', new DOMException('aborted', 'AbortError')],
    ['an Error renamed AbortError', Object.assign(new Error('aborted'), { name: 'AbortError' })],
    ['a plain object named AbortError', { name: 'AbortError' }],
  ])('accepts %s', (_label, error) => {
    expect(isAbortError(error)).toBe(true)
  })

  it.each([
    ['a TimeoutError', new DOMException('timed out', 'TimeoutError')],
    ['a TypeError', new TypeError('Failed to fetch')],
    ['the string AbortError', 'AbortError'],
    ['null', null],
    ['undefined', undefined],
  ])('rejects %s', (_label, error) => {
    expect(isAbortError(error)).toBe(false)
  })
})

describe('isTimeoutError', () => {
  it.each([
    ['a DOMException named TimeoutError', new DOMException('timed out', 'TimeoutError')],
    ['a plain object named TimeoutError', { name: 'TimeoutError' }],
  ])('accepts %s', (_label, error) => {
    expect(isTimeoutError(error)).toBe(true)
  })

  it.each([
    ['an AbortError', new DOMException('aborted', 'AbortError')],
    ['an Error', new Error('timed out')],
    ['null', null],
  ])('rejects %s', (_label, error) => {
    expect(isTimeoutError(error)).toBe(false)
  })
})
