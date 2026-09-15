import { describe, it, expect } from 'vitest'
import { isSameUserId, toAuthUser, type AuthApiUser } from './authUser'

// A payload in the shape `AuthApiUser` declares. That declaration is narrower
// than the wire for the string fields (see the type's doc: the nullable ones
// arrive as null), so these fixtures pin the mapping, not the contract. `id`
// is the exception: its declared union is the wire's, and the numeric case is
// covered on its own below.
const apiUser: AuthApiUser = {
  id: 'user-1',
  email: 'admin@test.local',
  username: 'reggie',
  display_name: 'Reggie',
  first_name: 'Reg',
  last_name: 'Gie',
  bio: 'bio text',
  location: 'Phoenix, AZ',
  avatar_url: 'https://example.test/a.png',
  is_admin: true,
  email_verified: true,
  user_tier: 'trusted_contributor',
  nav_mode: 'side',
}

describe('toAuthUser', () => {
  it('carries every context field through, privilege fields included', () => {
    expect(toAuthUser(apiUser)).toEqual({
      id: 'user-1',
      email: 'admin@test.local',
      username: 'reggie',
      display_name: 'Reggie',
      first_name: 'Reg',
      last_name: 'Gie',
      bio: 'bio text',
      location: 'Phoenix, AZ',
      avatar_url: 'https://example.test/a.png',
      is_admin: true,
      email_verified: true,
      user_tier: 'trusted_contributor',
      nav_mode: 'side',
    })
  })

  // The defect the coercion exists for: the backend serializes `id` as a JSON
  // number, the context declares it `string`, and a gate comparing a viewer id
  // to an entity's owner column then answers false for the very reader who
  // owns the row.
  it('narrows a numeric wire id to the string the context declares', () => {
    const mapped = toAuthUser({ ...apiUser, id: 42 })
    expect(mapped.id).toBe('42')
    expect(typeof mapped.id).toBe('string')
  })

  it('leaves a string id alone', () => {
    expect(toAuthUser({ ...apiUser, id: '42' }).id).toBe('42')
  })

  // `String(undefined)` is the truthy string 'undefined', which reads as a
  // viewer. An empty id is falsy, so every identity predicate denies.
  it('maps an absent id to an empty string, not a truthy placeholder', () => {
    const withoutId = { ...apiUser, id: undefined as unknown as string }
    expect(toAuthUser(withoutId).id).toBe('')
    expect(isSameUserId(toAuthUser(withoutId).id, 42)).toBe(false)
  })

  // The regression this function exists for: a hand-mapped session-entry
  // response reached the context with `is_admin` dropped and `email_verified`
  // stated as a placeholder. The `true` direction is covered above.
  it('reports a non-admin unverified viewer as exactly that', () => {
    const mapped = toAuthUser({ ...apiUser, is_admin: false, email_verified: false })
    expect(mapped.is_admin).toBe(false)
    expect(mapped.email_verified).toBe(false)
  })

  // `email_verified` is required on the context user, so an absent value needs
  // an answer. False is the one that gates rather than grants.
  it('falls back to unverified when the payload states nothing', () => {
    const withoutFlag: AuthApiUser = { ...apiUser }
    delete withoutFlag.email_verified
    expect(toAuthUser(withoutFlag).email_verified).toBe(false)
  })

  // `toStrictEqual`, not `toEqual`: the latter treats an absent key and an
  // explicit `undefined` as equal, so it would pass against a mapper that
  // silently stopped writing a field.
  it('writes every context key even when the payload carries only the required two', () => {
    expect(toAuthUser({ id: 'user-2', email: 'plain@test.local' })).toStrictEqual({
      id: 'user-2',
      email: 'plain@test.local',
      username: undefined,
      display_name: undefined,
      first_name: undefined,
      last_name: undefined,
      bio: undefined,
      location: undefined,
      avatar_url: undefined,
      is_admin: undefined,
      email_verified: false,
      user_tier: undefined,
      nav_mode: undefined,
    })
  })
})

describe('isSameUserId', () => {
  it('matches a string viewer id against a numeric owner column', () => {
    expect(isSameUserId('42', 42)).toBe(true)
    expect(isSameUserId(42, '42')).toBe(true)
    expect(isSameUserId('42', '42')).toBe(true)
    expect(isSameUserId(42, 42)).toBe(true)
  })

  it('refuses two different users', () => {
    expect(isSameUserId('42', 7)).toBe(false)
    expect(isSameUserId('7', 42)).toBe(false)
  })

  // Compared as strings, not as numbers: `Number(' 42 ')` is 42 and
  // `Number('')` is 0, so a numeric comparison accepts padding and a blank as
  // ids.
  it('refuses a spelling that is not identical', () => {
    expect(isSameUserId(' 42', 42)).toBe(false)
    expect(isSameUserId('42 ', 42)).toBe(false)
    expect(isSameUserId('42.0', 42)).toBe(false)
    expect(isSameUserId('0x2a', 42)).toBe(false)
  })

  it('refuses when either side has no id', () => {
    expect(isSameUserId(undefined, 42)).toBe(false)
    expect(isSameUserId(null, 42)).toBe(false)
    expect(isSameUserId('', 42)).toBe(false)
    expect(isSameUserId('42', undefined)).toBe(false)
    expect(isSameUserId('42', null)).toBe(false)
    expect(isSameUserId(undefined, undefined)).toBe(false)
    expect(isSameUserId('', '')).toBe(false)
  })

  // No user row has id 0, so a zero on either side is an absent id.
  it('refuses a zero id on either side', () => {
    expect(isSameUserId(0, 0)).toBe(false)
    expect(isSameUserId('0', 0)).toBe(false)
  })
})
