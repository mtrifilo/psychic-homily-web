import { describe, it, expect } from 'vitest'
import { toAuthUser, type AuthApiUser } from './authUser'

// The payload shape the auth API returns on /auth/profile and on every
// endpoint that establishes a session.
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

  // The regression this function exists for: an admin whose session-entry
  // response says is_admin, mapped by hand, arrived in the context without it,
  // and the override outranks the profile for the rest of the SPA session.
  it('preserves is_admin rather than dropping it', () => {
    expect(toAuthUser({ ...apiUser, is_admin: true }).is_admin).toBe(true)
    expect(toAuthUser({ ...apiUser, is_admin: false }).is_admin).toBe(false)
  })

  it('preserves email_verified rather than stating a placeholder', () => {
    expect(toAuthUser({ ...apiUser, email_verified: true }).email_verified).toBe(true)
    expect(toAuthUser({ ...apiUser, email_verified: false }).email_verified).toBe(false)
  })

  // `email_verified` is required on the context user, so an absent value needs
  // an answer. False is the one that gates rather than grants.
  it('falls back to unverified when the payload states nothing', () => {
    const withoutFlag: AuthApiUser = { ...apiUser }
    delete withoutFlag.email_verified
    expect(toAuthUser(withoutFlag).email_verified).toBe(false)
  })

  it('leaves absent optional fields absent', () => {
    expect(toAuthUser({ id: 'user-2', email: 'plain@test.local' })).toEqual({
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
