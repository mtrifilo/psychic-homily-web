import { describe, it, expect } from 'vitest'
import { toAuthUser, type AuthApiUser } from './authUser'

// A payload in the shape `AuthApiUser` declares. That declaration is narrower
// than the wire (see the type's doc: `id` arrives as a number, the nullable
// strings as null), so these fixtures pin the mapping, not the contract.
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

  // The regression this function exists for: a hand-mapped session-entry
  // response reached the context with `is_admin` dropped and `email_verified`
  // stated as a placeholder, and the override outranks the profile for the
  // rest of the SPA session. The `true` direction is covered above.
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
