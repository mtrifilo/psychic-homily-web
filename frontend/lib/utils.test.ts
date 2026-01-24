import { describe, it, expect } from 'vitest'
import { cn } from './utils'

describe('cn utility', () => {
  it('merges class names', () => {
    const result = cn('foo', 'bar')
    expect(result).toBe('foo bar')
  })

  it('handles undefined values', () => {
    const result = cn('foo', undefined, 'bar')
    expect(result).toBe('foo bar')
  })

  it('handles null values', () => {
    const result = cn('foo', null, 'bar')
    expect(result).toBe('foo bar')
  })

  it('handles boolean false values', () => {
    const result = cn('foo', false && 'baz', 'bar')
    expect(result).toBe('foo bar')
  })

  it('handles conditional classes', () => {
    const isActive = true
    const result = cn('base', isActive && 'active')
    expect(result).toBe('base active')
  })

  it('handles objects with boolean values', () => {
    const result = cn({
      foo: true,
      bar: false,
      baz: true,
    })
    expect(result).toBe('foo baz')
  })

  it('handles arrays of class names', () => {
    const result = cn(['foo', 'bar'], 'baz')
    expect(result).toBe('foo bar baz')
  })

  it('merges Tailwind conflicting classes (keeps last)', () => {
    const result = cn('p-4', 'p-2')
    expect(result).toBe('p-2')
  })

  it('merges Tailwind padding conflicts correctly', () => {
    const result = cn('px-4 py-2', 'p-6')
    expect(result).toBe('p-6')
  })

  it('merges Tailwind color conflicts correctly', () => {
    const result = cn('text-red-500', 'text-blue-500')
    expect(result).toBe('text-blue-500')
  })

  it('merges Tailwind background color conflicts', () => {
    const result = cn('bg-white', 'bg-gray-100')
    expect(result).toBe('bg-gray-100')
  })

  it('preserves non-conflicting classes', () => {
    const result = cn('p-4', 'm-2', 'text-red-500')
    expect(result).toBe('p-4 m-2 text-red-500')
  })

  it('handles empty string', () => {
    const result = cn('')
    expect(result).toBe('')
  })

  it('handles no arguments', () => {
    const result = cn()
    expect(result).toBe('')
  })

  it('handles complex nested inputs', () => {
    const result = cn('base', ['nested', 'array'], { object: true })
    expect(result).toBe('base nested array object')
  })

  it('handles whitespace correctly', () => {
    const result = cn('  foo  ', '  bar  ')
    expect(result).toBe('foo bar')
  })

  it('merges responsive variants correctly', () => {
    const result = cn('md:p-4', 'md:p-6')
    expect(result).toBe('md:p-6')
  })

  it('handles state variants', () => {
    const result = cn('hover:bg-blue-500', 'hover:bg-red-500')
    expect(result).toBe('hover:bg-red-500')
  })
})
