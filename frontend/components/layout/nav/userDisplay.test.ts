import { describe, it, expect } from 'vitest'
import { getUserInitials, getUserDisplayName } from './userDisplay'

describe('getUserInitials', () => {
  it('prefers display_name, deriving first+last-word initials (PSY-1063)', () => {
    expect(
      getUserInitials({
        display_name: 'Desert Lifer',
        first_name: 'John',
        email: 'j@test.com',
      })
    ).toBe('DL')
  })

  it('derives a single initial from a one-word display_name', () => {
    expect(
      getUserInitials({ display_name: 'Mononym', email: 'j@test.com' })
    ).toBe('M')
  })

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
  it('prefers display_name over first/last (PSY-1063)', () => {
    expect(
      getUserDisplayName({
        display_name: 'Desert Lifer',
        first_name: 'John',
        last_name: 'Doe',
      })
    ).toBe('Desert Lifer')
  })

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
