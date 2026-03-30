import { describe, it, expect } from 'vitest'
import { cn } from './utils'

describe('cn utility', () => {
  it('merges class names', () => {
    const result = cn('foo', 'bar')
    expect(result).toBe('foo bar')
  })

  it('handles undefined and null values', () => {
    expect(cn('foo', undefined, 'bar')).toBe('foo bar')
    expect(cn('foo', null, 'bar')).toBe('foo bar')
  })

  it('handles conditional classes via boolean expressions', () => {
    expect(cn('foo', false && 'baz', 'bar')).toBe('foo bar')
    expect(cn('base', true && 'active')).toBe('base active')
  })

  it('handles objects and arrays', () => {
    expect(cn({ foo: true, bar: false, baz: true })).toBe('foo baz')
    expect(cn(['foo', 'bar'], 'baz')).toBe('foo bar baz')
  })

  it('handles empty/no arguments', () => {
    expect(cn('')).toBe('')
    expect(cn()).toBe('')
  })
})
