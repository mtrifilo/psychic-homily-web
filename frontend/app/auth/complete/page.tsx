import { CreateAccountStep } from './create-account-step'

/**
 * Where the sign-in callback sends a new Google identity (oauthSignupStepPath
 * in backend/internal/api/handlers/auth/oauth_signup.go).
 */
export default function CompleteSignupPage() {
  return <CreateAccountStep />
}
