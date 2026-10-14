import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from '@simplewebauthn/browser'
import type { components } from '@/types/api'

type CredentialCreation = components['schemas']['CredentialCreation']
type CredentialAssertion = components['schemas']['CredentialAssertion']

/**
 * The passkey begin endpoints answer with the argument of
 * `navigator.credentials.create()` / `.get()`: the WebAuthn options sit under
 * `publicKey`. `@simplewebauthn/browser` takes the inner options as
 * `optionsJSON` and reads `challenge` off them directly, so the wrapper has to
 * come off before the ceremony starts.
 *
 * Each returns null when the options carry no challenge, so the caller can
 * report its own failure instead of the library throwing on a missing field.
 */
export function registrationOptionsFrom(
  options: CredentialCreation | undefined
): PublicKeyCredentialCreationOptionsJSON | null {
  if (!options?.publicKey?.challenge) return null
  return options.publicKey as PublicKeyCredentialCreationOptionsJSON
}

export function authenticationOptionsFrom(
  options: CredentialAssertion | undefined
): PublicKeyCredentialRequestOptionsJSON | null {
  if (!options?.publicKey?.challenge) return null
  return options.publicKey as PublicKeyCredentialRequestOptionsJSON
}
