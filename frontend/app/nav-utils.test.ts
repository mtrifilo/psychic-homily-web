import { describe, it, expect } from 'vitest'
import { getUserInitials, getUserDisplayName, isExternal, navLinks } from './nav-utils'

describe('getUserInitials', () => {
  it('returns first + last initials when both names are provided', () => {
    expect(getUserInitials({ first_name: 'John', last_name: 'Doe', email: 'j@test.com' })).toBe('JD')
  })

  it('returns first initial only when last name is missing', () => {
    expect(getUserInitials({ first_name: 'John', email: 'j@test.com' })).toBe('J')
  })

  it('returns first initial only when last name is empty', () => {
    expect(getUserInitials({ first_name: 'Jane', last_name: '', email: 'j@test.com' })).toBe('J')
  })

  it('returns first char of email when no first name', () => {
    expect(getUserInitials({ email: 'test@example.com' })).toBe('T')
  })

  it('uppercases the email initial', () => {
    expect(getUserInitials({ email: 'abc@example.com' })).toBe('A')
  })

  it('returns "?" when email is empty and no first name (bug fix)', () => {
    expect(getUserInitials({ email: '' })).toBe('?')
  })

  it('returns "?" when first_name is empty string and email is empty', () => {
    expect(getUserInitials({ first_name: '', email: '' })).toBe('?')
  })

  it('handles first_name as undefined with valid email', () => {
    expect(getUserInitials({ first_name: undefined, email: 'x@y.com' })).toBe('X')
  })
})

describe('getUserDisplayName', () => {
  it('returns full name when both first and last are provided', () => {
    expect(getUserDisplayName({ first_name: 'John', last_name: 'Doe' })).toBe('John Doe')
  })

  it('returns first name only when last name is missing', () => {
    expect(getUserDisplayName({ first_name: 'John' })).toBe('John')
  })

  it('returns null when neither name is provided', () => {
    expect(getUserDisplayName({})).toBeNull()
  })

  it('returns null when both are empty strings', () => {
    expect(getUserDisplayName({ first_name: '', last_name: '' })).toBeNull()
  })

  it('returns first name when last name is empty string', () => {
    expect(getUserDisplayName({ first_name: 'Alice', last_name: '' })).toBe('Alice')
  })
})

describe('isExternal', () => {
  it('returns true for a link with external: true', () => {
    const substackLink = navLinks.find(l => l.label === 'Substack')!
    expect(isExternal(substackLink)).toBe(true)
  })

  it('returns false for a standard internal link', () => {
    const showsLink = navLinks.find(l => l.label === 'Shows')!
    expect(isExternal(showsLink)).toBe(false)
  })

  it('returns false for all non-substack links', () => {
    const internalLinks = navLinks.filter(l => l.label !== 'Substack')
    for (const link of internalLinks) {
      expect(isExternal(link)).toBe(false)
    }
  })
})
