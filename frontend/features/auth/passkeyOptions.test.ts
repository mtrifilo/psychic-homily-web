import { describe, it, expect } from 'vitest'
import { authenticationOptionsFrom, registrationOptionsFrom } from './passkeyOptions'
import type { components } from '@/types/api'

type CredentialCreation = components['schemas']['CredentialCreation']
type CredentialAssertion = components['schemas']['CredentialAssertion']

describe('registrationOptionsFrom', () => {
  it('returns the options inside the publicKey wrapper', () => {
    const publicKey = {
      challenge: 'reg-challenge',
      rp: { name: 'Psychic Homily', id: 'localhost' },
      user: { id: 'AAAAAAAAAAI', name: 'a@b.c', displayName: 'A' },
    }
    expect(registrationOptionsFrom({ publicKey } as CredentialCreation)).toBe(publicKey)
  })

  it('returns null for missing options or a missing challenge', () => {
    expect(registrationOptionsFrom(undefined)).toBeNull()
    expect(
      registrationOptionsFrom({ publicKey: { challenge: '' } } as CredentialCreation)
    ).toBeNull()
    // The pre-unwrapped shape is not what the server sends; it carries no
    // publicKey and is refused rather than handed to the library.
    expect(
      registrationOptionsFrom({ challenge: 'x' } as unknown as CredentialCreation)
    ).toBeNull()
  })
})

describe('authenticationOptionsFrom', () => {
  it('returns the options inside the publicKey wrapper', () => {
    const publicKey = { challenge: 'auth-challenge', rpId: 'localhost' }
    expect(authenticationOptionsFrom({ publicKey } as CredentialAssertion)).toBe(publicKey)
  })

  it('returns null for missing options or a missing challenge', () => {
    expect(authenticationOptionsFrom(undefined)).toBeNull()
    expect(
      authenticationOptionsFrom({ publicKey: {} } as unknown as CredentialAssertion)
    ).toBeNull()
  })
})
