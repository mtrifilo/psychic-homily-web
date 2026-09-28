import { describe, it, expect } from 'vitest'
import {
  readProfilePreference,
  readProfileViewerId,
  settleProfilePreference,
  withProfilePreference,
} from './profilePreferenceCache'

function payload(id: string | number, preferences?: Record<string, unknown>) {
  return { success: true, user: { id, name: 'x', preferences } }
}

describe('profilePreferenceCache', () => {
  it('reads a preference off user.preferences, absent and null as null', () => {
    expect(readProfilePreference(payload(7, { a: 1 }), 'a')).toBe(1)
    expect(readProfilePreference(payload(7, { a: null }), 'a')).toBeNull()
    expect(readProfilePreference(payload(7), 'a')).toBeNull()
    expect(readProfilePreference(undefined, 'a')).toBeNull()
    expect(readProfileViewerId(payload('7'))).toBe('7')
  })

  it('replaces one preference and leaves the rest of the payload alone', () => {
    const next = withProfilePreference(payload(7, { a: 1, b: 2 }), 'a', 3)
    expect(next).toEqual(payload(7, { a: 3, b: 2 }))
  })

  it('leaves an entry that names no viewer untouched', () => {
    const signedOut = { success: false }
    expect(withProfilePreference(signedOut, 'a', 1)).toBe(signedOut)
  })

  // The guard the settle-time writes rely on: a request that outlives its
  // session must not write into the next account's payload.
  it('settles only into the viewer the write started for', () => {
    const same = settleProfilePreference(payload(7, { a: 1 }), 7, 'a', 2)
    expect(readProfilePreference(same, 'a')).toBe(2)

    const other = payload(8, { a: 1 })
    expect(settleProfilePreference(other, 7, 'a', 2)).toBe(other)
  })
})
