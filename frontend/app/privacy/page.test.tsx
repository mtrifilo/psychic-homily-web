import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CURRENT_PRIVACY_VERSION } from '@/lib/legal'
import PrivacyPolicyPage from './page'

// The version every sign-up records stands for the text this page shows, so
// the page's effective date is the pinned privacy version.
function longDate(isoDate: string): string {
  return new Date(`${isoDate}T00:00:00Z`).toLocaleDateString('en-US', {
    timeZone: 'UTC',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

describe('PrivacyPolicyPage (app/privacy)', () => {
  it('shows the pinned privacy version as its effective date', () => {
    render(<PrivacyPolicyPage />)
    expect(
      screen.getByText(`Effective: ${longDate(CURRENT_PRIVACY_VERSION)}`, {
        exact: false,
      })
    ).toBeInTheDocument()
  })

  it('names the date of birth, its purpose and that it is never shown', () => {
    render(<PrivacyPolicyPage />)
    const item = screen.getByText('Date of Birth:').closest('li')
    expect(item).toHaveTextContent(
      'Date of Birth: Asked for when you create an account and used only to confirm you are at least 16. It is stored with your account, never shown on your profile or to other users, and included when you export your data.'
    )
  })
})
