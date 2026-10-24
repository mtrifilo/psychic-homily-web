import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { renderWithProviders } from '@/test/utils'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { deferred } from '@/test/inflightOrdering'
import {
  NEWER_VERSIONS_BODY,
  publishLegalVersionsInTurn,
  publishedVersions,
} from '@/test/consentVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { CONSENT_VERSIONS_CHANGED } from '@/app/auth/_components/consent-versions-notice'
import {
  CreateAccountStep,
  DATE_OF_BIRTH_INVALID_MESSAGE,
  TERMS_REQUIRED_MESSAGE,
  dateOfBirthFromParts,
} from './create-account-step'

vi.mock('@/lib/api-base', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  OAUTH_BACKEND_URL: 'https://oauth-backend.test',
}))

// The step leaves by replace, so its spent URL never stays in history.
const mockReplace = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: mockReplace }),
  useSearchParams: () => new URLSearchParams(),
}))

const mockSetUser = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

const PENDING = {
  status: 'pending',
  email: 'alice@example.com',
  name: 'Alice Moreno',
  expires_at: '2026-10-23T12:10:00Z',
}

const NEW_USER = {
  id: 7,
  email: 'alice@example.com',
  display_name: 'Alice Moreno',
  email_verified: true,
}

function answerPending(body: unknown, status = 200) {
  server.use(
    http.get(`${TEST_API_BASE}/auth/oauth/pending-signup`, () =>
      HttpResponse.json(body as Record<string, unknown>, { status })
    )
  )
}

/** Records each finish body and answers with answer. */
function answerFinish(answer: () => Record<string, unknown>) {
  const bodies: Array<Record<string, unknown>> = []
  server.use(
    http.post(`${TEST_API_BASE}/auth/oauth/finish-signup`, async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>)
      return HttpResponse.json(answer())
    }),
    // The profile refetch a new session triggers.
    http.get(`${TEST_API_BASE}/auth/profile`, () =>
      HttpResponse.json({ success: true, message: 'ok', user: NEW_USER })
    )
  )
  return bodies
}

async function fillValidForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Month'), '4')
  await user.type(screen.getByLabelText('Day'), '27')
  await user.type(screen.getByLabelText('Year'), '1998')
  await user.click(screen.getByRole('checkbox', { name: /I agree to the Terms of Service/ }))
}

describe('dateOfBirthFromParts', () => {
  it.each([
    ['4', '27', '1998', '1998-04-27'],
    ['04', '07', '2000', '2000-04-07'],
    ['2', '29', '2000', '2000-02-29'],
  ])('reads %s/%s/%s as %s', (m, d, y, want) => {
    expect(dateOfBirthFromParts(m, d, y)).toBe(want)
  })

  it.each([
    ['2', '29', '1999'],
    ['2', '30', '2000'],
    ['13', '1', '2000'],
    ['0', '1', '2000'],
    ['1', '0', '2000'],
    ['1', '1', '98'],
    ['', '1', '2000'],
    ['1', '', '2000'],
    ['1', '1', ''],
  ])('refuses %s/%s/%s', (m, d, y) => {
    expect(dateOfBirthFromParts(m, d, y)).toBeNull()
  })
})

describe('CreateAccountStep', () => {
  beforeEach(() => {
    mockReplace.mockReset()
    mockSetUser.mockReset()
  })

  it('spins while it reads the pending sign-up', () => {
    const pending = deferred<Response>()
    server.use(
      http.get(`${TEST_API_BASE}/auth/oauth/pending-signup`, () => pending.promise)
    )
    renderWithProviders(<CreateAccountStep />)

    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
  })

  it('renders the step as drawn, with the Google address read-only and the name prefilled', async () => {
    answerPending(PENDING)
    renderWithProviders(<CreateAccountStep />)

    expect(await screen.findByRole('heading', { name: 'Create your Psychic Homily account' })).toBeInTheDocument()
    expect(screen.getByText('Last step')).toBeInTheDocument()
    expect(screen.getByText('Check your details, then agree to the Terms to finish.')).toBeInTheDocument()
    expect(screen.getByText('alice@example.com')).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /Google account/ })).not.toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Alice Moreno')
    for (const part of ['Month', 'Day', 'Year']) {
      expect(screen.getByLabelText(part)).toHaveValue('')
    }
    expect(screen.getByText('Never shown on your profile.')).toBeInTheDocument()
    const terms = screen.getByRole('checkbox', { name: /I agree to the Terms of Service/ })
    expect(terms).not.toBeChecked()
    expect(terms).toHaveAccessibleName(
      'I agree to the Terms of Service and I acknowledge the Privacy Policy'
    )
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
    expect(
      screen.getByText(/^By clicking Create account you agree to the/)
    ).toHaveTextContent(
      'By clicking Create account you agree to the Terms of Service and acknowledge the Privacy Policy.'
    )
  })

  it('offers Google again with the account chooser for a different account, keeping the destination', async () => {
    answerPending({ ...PENDING, return_to: '/shows?city=phoenix' })
    renderWithProviders(<CreateAccountStep />)

    const link = await screen.findByRole('link', { name: 'Use a different account' })
    const href = new URL(link.getAttribute('href')!)
    expect(href.origin + href.pathname).toBe('https://oauth-backend.test/auth/login/google')
    expect(href.searchParams.get('prompt')).toBe('select_account')
    expect(href.searchParams.get('returnTo')).toBe('/shows?city=phoenix')
  })

  it('says to agree to the Terms, and sends nothing, when the box is unticked', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await user.type(screen.getByLabelText('Month'), '4')
    await user.type(screen.getByLabelText('Day'), '27')
    await user.type(screen.getByLabelText('Year'), '1998')
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    const terms = screen.getByRole('checkbox', { name: /I agree to the Terms of Service/ })
    expect(screen.getByText(TERMS_REQUIRED_MESSAGE)).toBeInTheDocument()
    expect(terms).toHaveAttribute('aria-invalid', 'true')
    expect(terms).toHaveAccessibleDescription(TERMS_REQUIRED_MESSAGE)
    expect(terms).toHaveFocus()
    expect(bodies).toHaveLength(0)

    await user.click(terms)
    expect(screen.queryByText(TERMS_REQUIRED_MESSAGE)).not.toBeInTheDocument()
  })

  it('sends nothing for a date that is not a date', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await user.type(screen.getByLabelText('Month'), '2')
    await user.type(screen.getByLabelText('Day'), '30')
    await user.type(screen.getByLabelText('Year'), '1998')
    await user.click(screen.getByRole('checkbox', { name: /I agree to the Terms of Service/ }))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(screen.getByText(DATE_OF_BIRTH_INVALID_MESSAGE)).toBeInTheDocument()
    expect(screen.getByLabelText('Month')).toHaveFocus()
    expect(bodies).toHaveLength(0)
  })

  it('creates the account with the consent, the date and the name, then goes where the sign-in was headed', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({
      success: true,
      message: 'Account created',
      user: NEW_USER,
      return_to: '/shows?city=phoenix',
    }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await user.clear(screen.getByLabelText('Name'))
    await user.type(screen.getByLabelText('Name'), 'Alice M.')
    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith('/shows?city=phoenix'))
    expect(bodies).toEqual([
      {
        terms_accepted: true,
        terms_version: CURRENT_TERMS_VERSION,
        privacy_version: CURRENT_PRIVACY_VERSION,
        date_of_birth: '1998-04-27',
        name: 'Alice M.',
      },
    ])
    expect(mockSetUser).toHaveBeenCalledWith(NEW_USER)
  })

  it.each([
    ['no destination', undefined, '/'],
    ['a destination the sign-in page refuses', '//evil.example', '/'],
  ])('lands on the site root for %s', async (_label, returnTo, want) => {
    answerPending(PENDING)
    answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER, return_to: returnTo }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith(want))
  })

  it('shows the under-16 refusal in place of the form when the finish refuses the age', async () => {
    answerPending(PENDING)
    answerFinish(() => ({
      success: false,
      message: 'You must confirm that you are at least 16 years old to create an account.',
      error_code: 'AGE_CONFIRMATION_REQUIRED',
    }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    const heading = await screen.findByRole('heading', { name: "We can't create an account for you." })
    assertAgeRefusalCard()
    expect(heading).toHaveFocus()
    expect(screen.queryByLabelText('Month')).not.toBeInTheDocument()
    expect(mockReplace).not.toHaveBeenCalled()
  })

  function assertAgeRefusalCard() {
    expect(screen.getByText('Account not created')).toBeInTheDocument()
    expect(
      screen.getByText(
        "Psychic Homily accounts are for people 16 and over, so we haven't created one. You can still browse shows, artists and venues without an account."
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Browse upcoming shows' })).toHaveAttribute('href', '/shows')
  }

  it('shows the refusal, without the form, to a browser already refused for its age', async () => {
    answerPending({ status: 'age_refused' })
    renderWithProviders(<CreateAccountStep />)

    expect(await screen.findByRole('heading', { name: "We can't create an account for you." })).toBeInTheDocument()
    assertAgeRefusalCard()
    expect(screen.queryByLabelText('Month')).not.toBeInTheDocument()
  })

  function assertExpiredCard() {
    expect(screen.getByText('Sign-in expired')).toBeInTheDocument()
    expect(
      screen.getByText(
        'This step stays open for 10 minutes and works once. Nothing was saved, so continue with Google to pick up where you left off.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Continue with Google' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'sign in' })).toHaveAttribute('href', '/auth')
  }

  it('shows the expired step for a pending sign-up that is gone', async () => {
    answerPending({ status: 'expired' })
    renderWithProviders(<CreateAccountStep />)

    expect(await screen.findByRole('heading', { name: 'That sign-in link expired. Start again.' })).toBeInTheDocument()
    assertExpiredCard()
  })

  it('shows the expired step when the finish finds the step used or expired', async () => {
    answerPending(PENDING)
    answerFinish(() => ({ success: false, message: 'gone', error_code: 'OAUTH_SIGNUP_EXPIRED' }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(await screen.findByRole('heading', { name: 'That sign-in link expired. Start again.' })).toHaveFocus()
    assertExpiredCard()
  })

  it("shows any other refusal in the finish's own words above the form", async () => {
    answerPending(PENDING)
    answerFinish(() => ({ success: false, message: 'Name must be 100 characters or fewer', error_code: 'VALIDATION_FAILED' }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Name must be 100 characters or fewer')
    expect(screen.getByLabelText('Month')).toBeInTheDocument()
    expect(mockReplace).not.toHaveBeenCalled()
  })

  it('asks for a reload, and sends nothing, when the published versions are not this build\'s', async () => {
    answerPending(PENDING)
    publishLegalVersionsInTurn(() => publishedVersions(NEWER_VERSIONS_BODY))
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    expect(await screen.findByText(CONSENT_VERSIONS_CHANGED)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create account' })).toBeDisabled()
    expect(bodies).toHaveLength(0)
  })

  it('keeps the provider names by sending no name when the prefill is untouched', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).not.toHaveProperty('name')
  })

  it('sends a cleared name, so the account gets no name', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await user.clear(screen.getByLabelText('Name'))
    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0].name).toBe('')
  })

  it('stays busy once the account exists, while the page leaves', async () => {
    answerPending(PENDING)
    const bodies = answerFinish(() => ({ success: true, message: 'ok', user: NEW_USER }))
    const user = userEvent.setup()
    renderWithProviders(<CreateAccountStep />)
    await screen.findByLabelText('Name')

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => expect(mockReplace).toHaveBeenCalled())
    const button = screen.getByRole('button', { name: /Creating account/ })
    expect(button).toBeDisabled()
    expect(bodies).toHaveLength(1)
  })

  it('carries the destination into Continue with Google on the expired step after a submit', async () => {
    answerPending({ ...PENDING, return_to: '/shows?city=phoenix' })
    answerFinish(() => ({ success: false, message: 'gone', error_code: 'OAUTH_SIGNUP_EXPIRED' }))
    const navigatedTo = vi.fn<(href: string) => void>()
    const originalLocation = window.location
    Object.defineProperty(window, 'location', { value: { ...originalLocation }, writable: true, configurable: true })
    Object.defineProperty(window.location, 'href', { set: navigatedTo, configurable: true })
    try {
      const user = userEvent.setup()
      renderWithProviders(<CreateAccountStep />)
      await screen.findByLabelText('Name')
      await fillValidForm(user)
      await user.click(screen.getByRole('button', { name: 'Create account' }))
      await user.click(await screen.findByRole('button', { name: 'Continue with Google' }))

      expect(new URL(navigatedTo.mock.calls[0][0]).searchParams.get('returnTo')).toBe('/shows?city=phoenix')
    } finally {
      Object.defineProperty(window, 'location', { value: originalLocation, writable: true, configurable: true })
    }
  })

  it('says the step could not load when the read fails', async () => {
    answerPending({ detail: 'unavailable' }, 503)
    renderWithProviders(<CreateAccountStep />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Service temporarily unavailable. Please try again.'
    )
  })
})
