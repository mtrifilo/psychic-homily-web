import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import {
  BUILD_CONSENT_VERSIONS,
  BUILD_VERSIONS_BODY,
  NEWER_VERSIONS_BODY,
  clientWithConsentVersions,
  publishedVersions,
  versionsUnavailable,
} from '@/test/consentVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { deferred } from '@/test/inflightOrdering'
import {
  CONSENT_RETRY_NOTICE,
  CONSENT_VERSIONS_CHANGED,
  CONSENT_VERSIONS_UNAVAILABLE,
} from './consent-versions-notice'
import {
  advanceFakeTime,
  requestSignal,
  stalledRequest,
  stalledResponseBody,
} from '@/test/stalledFetch'
import { PasskeySignupButton } from './passkey-signup'
import {
  SIGNUP_REQUEST_TIMEOUT_MS,
  useSignupRequestDeadline,
} from './signup-request-deadline'
import { ADULT_DATE_OF_BIRTH, fillDateOfBirth } from '@/test/dateOfBirth'
import {
  DATE_OF_BIRTH_INVALID_MESSAGE,
  DATE_OF_BIRTH_MISSING_MESSAGE,
} from './date-of-birth-field'

const mockSetUser = vi.fn()
const mockMarkAccountCreated = vi.fn()
vi.mock('@/features/auth/accountCreatedMarker', () => ({
  markAccountCreatedThisSession: (...args: unknown[]) => mockMarkAccountCreated(...args),
}))
const mockOnHandoffChange = vi.fn()
const mockOnDeadlineExpire = vi.fn()
const mockOnError = vi.fn()
const mockOnAgeRefused = vi.fn()
const mockStartRegistration = vi.fn()
const fetchMock = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startRegistration: (...args: unknown[]) => mockStartRegistration(...args),
}))

// The components must resolve the backend through lib/api-base like every
// other data request, never from NEXT_PUBLIC_API_URL on their own: unset, that
// read skips the same-origin /api proxy.
vi.mock('@/lib/api-base', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  API_BASE_URL: 'https://api-base.test',
}))

describe('PasskeySignupButton', () => {
  beforeEach(() => {
    mockSetUser.mockReset()
    mockMarkAccountCreated.mockReset()
    mockOnHandoffChange.mockReset()
    mockOnDeadlineExpire.mockReset()
    mockOnError.mockReset()
    mockOnAgeRefused.mockReset()
    mockStartRegistration.mockReset()
    fetchMock.mockReset()

    mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // Stands in for the page, which owns the deadline in production.
  function PasskeySignup({ versionsChanged }: { versionsChanged?: boolean }) {
    const requestDeadline = useSignupRequestDeadline(mockOnDeadlineExpire)
    return (
      <PasskeySignupButton
        onHandoffChange={mockOnHandoffChange}
        onError={mockOnError}
        onAgeRefused={mockOnAgeRefused}
        requestDeadline={requestDeadline}
        versionsChanged={versionsChanged}
      />
    )
  }

  // Seeded by default, so the fetch calls a test counts are the sign-up's own.
  function renderButton(queryClient = clientWithConsentVersions()) {
    return renderWithProviders(<PasskeySignup />, { queryClient })
  }

  async function submitSignup() {
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
    await fillDateOfBirth(user)
    await user.click(
      screen.getByRole('button', { name: /continue with passkey/i })
    )
  }

  function signupDialog() {
    return screen.getByRole('dialog', { name: 'Create account with passkey' })
  }

  // An error raised while the dialog is open shows inside it, and the page is
  // never handed it.
  async function expectDialogError(message: string) {
    await waitFor(() => {
      expect(within(signupDialog()).getByRole('alert')).toHaveTextContent(message)
    })
    expect(mockOnError).not.toHaveBeenCalled()
  }

  const BEGIN_OK = {
    json: async () => ({
      success: true,
      options: { publicKey: { challenge: 'begin-signup-challenge' } },
      challenge_id: 'challenge-id',
    }),
  }

  it('hands the new session to the page: pending before finish, then complete', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        user: {
          id: 100,
          email: 'signup@example.com',
          first_name: 'Signup',
          last_name: 'User',
          email_verified: false,
        },
      }),
    })

    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    const beginSignupPayload = JSON.parse(
      (fetchMock.mock.calls[0]?.[1] as { body: string }).body
    )
    expect(beginSignupPayload).toMatchObject({
      email: 'signup@example.com',
      terms_accepted: true,
      terms_version: CURRENT_TERMS_VERSION,
      privacy_version: CURRENT_PRIVACY_VERSION,
      date_of_birth: ADULT_DATE_OF_BIRTH.sent,
    })
    expect(beginSignupPayload).not.toHaveProperty('age_confirmed')
    expect(beginSignupPayload).not.toHaveProperty('min_age_attested')

    expect(mockStartRegistration).toHaveBeenCalledWith({
      optionsJSON: { challenge: 'begin-signup-challenge' },
    })
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      'https://api-base.test/auth/passkey/signup/begin',
      'https://api-base.test/auth/passkey/signup/finish',
    ])

    const finishSignupPayload = JSON.parse(
      (fetchMock.mock.calls[1]?.[1] as { body: string }).body
    )
    expect(finishSignupPayload).toMatchObject({ challenge_id: 'challenge-id', terms_accepted: true })
    expect(finishSignupPayload).not.toHaveProperty('date_of_birth')
    expect(finishSignupPayload).not.toHaveProperty('age_confirmed')

    await waitFor(() => {
      expect(mockOnHandoffChange).toHaveBeenCalledTimes(2)
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [
        {
          status: 'complete',
          email: 'signup@example.com',
          method: 'passkey',
          step: 'backup-prompt',
        },
      ],
    ])
    expect(mockSetUser).toHaveBeenCalledWith(
      expect.objectContaining({ id: 100, email: 'signup@example.com' })
    )
    expect(mockMarkAccountCreated).toHaveBeenCalledWith(100)
    await waitFor(() => {
      expect(
        screen.queryByRole('dialog', { name: 'Create account with passkey' })
      ).not.toBeInTheDocument()
    })
    // The pending claim is in place before the finish request goes out.
    expect(mockOnHandoffChange.mock.invocationCallOrder[0]).toBeLessThan(
      fetchMock.mock.invocationCallOrder[1]
    )
  })

  it('releases its claim and reports an error when finish returns no user', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: true }),
    })

    renderButton()
    await submitSignup()

    await expectDialogError('Failed to complete passkey signup')
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
    expect(mockMarkAccountCreated).not.toHaveBeenCalled()
  })

  it('releases its claim and reports the error when finish is refused', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()

    await expectDialogError('Email already registered')
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
  })

  // The envelope the backend writes for a failure: a 503 whose body says so.
  it('releases its claim, reports a 503 from finish and leaves the form usable', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 503,
      json: async () => ({
        success: false,
        message: 'Service temporarily unavailable',
        error_code: 'SERVICE_UNAVAILABLE',
      }),
    })

    renderButton()
    await submitSignup()

    await expectDialogError('Service temporarily unavailable')
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /continue with passkey/i })
      ).toBeEnabled()
    })
    expect(screen.getByLabelText('Email')).toHaveValue('signup@example.com')
  })

  it('claims nothing and reports the error when begin is refused', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()

    await expectDialogError('Email already registered')
    expect(mockStartRegistration).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(mockOnHandoffChange).not.toHaveBeenCalled()
  })

  it('claims nothing when the passkey ceremony is cancelled', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    mockStartRegistration.mockRejectedValueOnce(
      Object.assign(new Error('cancelled'), { name: 'NotAllowedError' })
    )

    renderButton()
    await submitSignup()

    // Re-enabled only once the catch and finally blocks have run.
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /continue with passkey/i })
      ).toBeEnabled()
    })
    expect(mockStartRegistration).toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(mockOnHandoffChange).not.toHaveBeenCalled()
    expect(mockOnError).not.toHaveBeenCalled()
  })

  it('shows an account-exists refusal in the open dialog and returns focus to Email', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: false,
        message: 'An account with this email already exists',
      }),
    })

    renderButton()
    await submitSignup()

    await expectDialogError('An account with this email already exists')
    const dialog = signupDialog()
    expect(within(dialog).getByLabelText('Email')).toHaveFocus()
    expect(within(dialog).getByLabelText('Email')).toBeEnabled()
    expect(within(dialog).getByLabelText('Email')).toHaveValue('signup@example.com')
    expect(
      within(dialog).getByRole('button', { name: /continue with passkey/i })
    ).toBeEnabled()
  })

  it('drops the dialog error when the next attempt starts', async () => {
    const retried = deferred<{ json: () => Promise<unknown> }>()
    fetchMock
      .mockResolvedValueOnce({
        json: async () => ({ success: false, message: 'Email already registered' }),
      })
      .mockReturnValueOnce(retried.promise)

    renderButton()
    await submitSignup()
    await expectDialogError('Email already registered')

    fireEvent.click(
      within(signupDialog()).getByRole('button', { name: /continue with passkey/i })
    )

    await waitFor(() => {
      expect(within(signupDialog()).queryByRole('alert')).not.toBeInTheDocument()
    })
    await act(async () => {
      retried.resolve(BEGIN_OK)
    })
  })

  it('reopens without the error an earlier attempt showed', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()
    await expectDialogError('Email already registered')

    fireEvent.keyDown(signupDialog(), { key: 'Escape' })
    await waitFor(() => {
      expect(
        screen.queryByRole('dialog', { name: 'Create account with passkey' })
      ).not.toBeInTheDocument()
    })
    fireEvent.click(screen.getByRole('button', { name: /sign up with passkey/i }))

    expect(within(signupDialog()).queryByRole('alert')).not.toBeInTheDocument()
    expect(within(signupDialog()).getByLabelText('Email')).toHaveValue('signup@example.com')
  })

  it('hands the page an error that arrives after the dialog was closed', async () => {
    const finish = deferred<{ json: () => Promise<unknown> }>()
    fetchMock.mockResolvedValueOnce(BEGIN_OK).mockReturnValueOnce(finish.promise)

    renderButton()
    await submitSignup()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    fireEvent.keyDown(signupDialog(), { key: 'Escape' })
    await waitFor(() => {
      expect(
        screen.queryByRole('dialog', { name: 'Create account with passkey' })
      ).not.toBeInTheDocument()
    })

    await act(async () => {
      finish.resolve({
        json: async () => ({ success: false, message: 'Email already registered' }),
      })
    })

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Email already registered')
    })
    expect(mockOnError).toHaveBeenCalledTimes(1)
  })

  describe('under the request deadline', () => {
    // fireEvent rather than userEvent: no focus moves, so nothing but this
    // component schedules work while the fake clock runs.
    function submitWithFireEvent() {
      fireEvent.click(screen.getByRole('button', { name: /sign up with passkey/i }))
      fireEvent.change(screen.getByLabelText('Email'), {
        target: { value: 'signup@example.com' },
      })
      fireEvent.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
      fireEvent.change(screen.getByLabelText('Month'), { target: { value: ADULT_DATE_OF_BIRTH.month } })
      fireEvent.change(screen.getByLabelText('Day'), { target: { value: ADULT_DATE_OF_BIRTH.day } })
      fireEvent.change(screen.getByLabelText('Year'), { target: { value: ADULT_DATE_OF_BIRTH.year } })
      fireEvent.click(screen.getByRole('button', { name: /continue with passkey/i }))
    }

    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: true })
    })

    afterEach(() => {
      vi.useRealTimers()
    })

    it.each([
      ['the finish response', stalledRequest],
      ['the finish response body', stalledResponseBody],
    ])(
      'releases its claim and reports the generic failure when %s stalls for 30 s',
      async (_label, finish) => {
        fetchMock.mockResolvedValueOnce(BEGIN_OK)
        fetchMock.mockImplementationOnce(finish)

        renderButton()
        submitWithFireEvent()
        await waitFor(() => {
          expect(fetchMock).toHaveBeenCalledTimes(2)
        })

        // The fake clock also follows real time, so the lead is not exact.
        await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS - 1_000)
        expect(requestSignal(fetchMock, 1).aborted).toBe(false)
        expect(mockOnHandoffChange.mock.calls).toEqual([
          [{ status: 'pending', email: 'signup@example.com' }],
        ])
        expect(mockOnError).not.toHaveBeenCalled()

        await advanceFakeTime(1_000)
        await expectDialogError('Failed to complete passkey signup')
        expect(requestSignal(fetchMock, 1).aborted).toBe(true)
        expect(mockOnDeadlineExpire).toHaveBeenCalledTimes(1)
        expect(mockOnHandoffChange.mock.calls).toEqual([
          [{ status: 'pending', email: 'signup@example.com' }],
          [null],
        ])
        expect(mockSetUser).not.toHaveBeenCalled()
        expect(
          screen.getByRole('button', { name: /continue with passkey/i })
        ).toBeEnabled()
        expect(screen.getByLabelText('Email')).toHaveValue('signup@example.com')
      }
    )

    it.each([
      ['the begin response', stalledRequest],
      ['the begin response body', stalledResponseBody],
    ])('claims nothing and reports the start failure when %s stalls for 30 s', async (_label, begin) => {
      fetchMock.mockImplementationOnce(begin)

      renderButton()
      submitWithFireEvent()
      await waitFor(() => {
        expect(fetchMock).toHaveBeenCalledTimes(1)
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS)
      await expectDialogError('Failed to start passkey signup')
      expect(requestSignal(fetchMock, 0).aborted).toBe(true)
      expect(mockStartRegistration).not.toHaveBeenCalled()
      expect(mockOnHandoffChange).not.toHaveBeenCalled()
      expect(
        screen.getByRole('button', { name: /continue with passkey/i })
      ).toBeEnabled()
    })

    it('disarms the deadline when finish is refused before it passes', async () => {
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      fetchMock.mockResolvedValueOnce({
        json: async () => ({ success: false, message: 'Email already registered' }),
      })

      renderButton()
      submitWithFireEvent()
      await expectDialogError('Email already registered')

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
      expect(requestSignal(fetchMock, 1).aborted).toBe(false)
      expect(mockOnDeadlineExpire).not.toHaveBeenCalled()
      expect(within(signupDialog()).getByRole('alert')).toHaveTextContent(
        'Email already registered'
      )
      expect(mockOnError).not.toHaveBeenCalled()
    })

    it('gives the passkey ceremony no deadline', async () => {
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      mockStartRegistration.mockReturnValueOnce(new Promise(() => {}))

      renderButton()
      submitWithFireEvent()
      await waitFor(() => {
        expect(mockStartRegistration).toHaveBeenCalled()
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 4)
      expect(requestSignal(fetchMock, 0).aborted).toBe(false)
      expect(mockOnDeadlineExpire).not.toHaveBeenCalled()
      expect(mockOnError).not.toHaveBeenCalled()
      expect(mockOnHandoffChange).not.toHaveBeenCalled()
    })

    it('leaves a completed sign-up alone once the deadline would have passed', async () => {
      fetchMock.mockResolvedValueOnce(BEGIN_OK)
      fetchMock.mockResolvedValueOnce({
        json: async () => ({
          success: true,
          user: { id: 100, email: 'signup@example.com', email_verified: false },
        }),
      })

      renderButton()
      submitWithFireEvent()
      await waitFor(() => {
        expect(mockOnHandoffChange).toHaveBeenCalledTimes(2)
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
      expect(requestSignal(fetchMock, 1).aborted).toBe(false)
      expect(mockOnHandoffChange.mock.calls).toEqual([
        [{ status: 'pending', email: 'signup@example.com' }],
        [
          {
            status: 'complete',
            email: 'signup@example.com',
            method: 'passkey',
            step: 'backup-prompt',
          },
        ],
      ])
      expect(mockOnError).not.toHaveBeenCalled()
      expect(mockOnDeadlineExpire).not.toHaveBeenCalled()
    })
  })

  it('opens with the date of birth in place of an age checkbox, and the Terms label as drawn', async () => {
    const user = userEvent.setup()
    renderButton()

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))

    const dialog = signupDialog()
    expect(within(dialog).getByRole('checkbox', { name: /Terms of Service/ })).toHaveAccessibleName(
      'I agree to the Terms of Service and I acknowledge the Privacy Policy'
    )
    expect(within(dialog).getByRole('group', { name: 'Date of birth' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('checkbox', { name: /years old/ })).not.toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: /continue with passkey/i })).toBeEnabled()
  })

  // Continue stays enabled; pressing it with fields missing shows every
  // field's error, focuses the first, and sends nothing.
  it('shows every field error on Continue with nothing filled, and focuses Email', async () => {
    const user = userEvent.setup()
    renderButton()

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.click(screen.getByRole('button', { name: /continue with passkey/i }))

    const dialog = signupDialog()
    expect(within(dialog).getByText('Please enter a valid email address')).toBeInTheDocument()
    expect(
      within(dialog).getByText('You must agree to the Terms of Service and Privacy Policy')
    ).toBeInTheDocument()
    expect(within(dialog).getByText(DATE_OF_BIRTH_MISSING_MESSAGE)).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Email')).toHaveFocus()
    expect(within(dialog).getByLabelText('Email')).toHaveAttribute('aria-invalid', 'true')
    expect(within(dialog).getByRole('button', { name: /continue with passkey/i })).toBeEnabled()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('focuses the date of birth, and sends nothing, when it is the only field left', async () => {
    const user = userEvent.setup()
    renderButton()

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
    await user.click(screen.getByRole('button', { name: /continue with passkey/i }))

    const dialog = signupDialog()
    expect(within(dialog).getByText(DATE_OF_BIRTH_MISSING_MESSAGE)).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Month')).toHaveFocus()
    for (const label of ['Month', 'Day', 'Year']) {
      expect(within(dialog).getByLabelText(label)).toHaveAttribute('aria-invalid', 'true')
    }
    expect(fetchMock).not.toHaveBeenCalled()

    // The error follows the field as it changes.
    await user.type(within(dialog).getByLabelText('Month'), '2')
    await user.type(within(dialog).getByLabelText('Day'), '30')
    await user.type(within(dialog).getByLabelText('Year'), '2004')
    expect(within(dialog).getByText(DATE_OF_BIRTH_INVALID_MESSAGE)).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Month')).toHaveAttribute('aria-invalid', 'false')
    expect(within(dialog).getByLabelText('Day')).toHaveAttribute('aria-invalid', 'true')
  })

  it('closes the dialog and hands the page the refusal when finish refuses a browser marked meanwhile', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: false,
        error_code: 'AGE_CONFIRMATION_REQUIRED',
        message: 'You must confirm that you are at least 16 years old to create an account.',
      }),
    })
    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnAgeRefused).toHaveBeenCalledTimes(1)
    })
    expect(
      screen.queryByRole('dialog', { name: 'Create account with passkey' })
    ).not.toBeInTheDocument()
    expect(mockOnError).not.toHaveBeenCalled()
    // The claim taken before finish is released.
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
  })

  it('closes the dialog and hands the page the refusal when begin refuses the age', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: false,
        error_code: 'AGE_CONFIRMATION_REQUIRED',
        message: 'You must confirm that you are at least 16 years old to create an account.',
      }),
    })
    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnAgeRefused).toHaveBeenCalledTimes(1)
    })
    expect(
      screen.queryByRole('dialog', { name: 'Create account with passkey' })
    ).not.toBeInTheDocument()
    expect(mockStartRegistration).not.toHaveBeenCalled()
    expect(mockOnError).not.toHaveBeenCalled()
    expect(mockOnHandoffChange).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  // Begin sends the versions GET /legal/versions publishes, never the build
  // constants, and a consent refusal of them is retried once with versions
  // read again past every cache.
  describe('consent versions', () => {
    const NEWER = {
      termsVersion: NEWER_VERSIONS_BODY.terms_version,
      privacyVersion: NEWER_VERSIONS_BODY.privacy_version,
    }
    const CONSENT_REFUSAL = {
      json: async () => ({
        success: false,
        error_code: 'TERMS_ACCEPTANCE_REQUIRED',
        message: 'Please accept the Terms of Service and Privacy Policy before creating an account.',
      }),
    }
    const FINISH_OK = {
      json: async () => ({
        success: true,
        user: { id: 100, email: 'signup@example.com', email_verified: false },
      }),
    }

    /**
     * Routes the versions read apart from the sign-up's own requests, which
     * take the queued answers in order; a request past the queue fails.
     */
    function routeVersions(
      answer: () => Promise<Response>,
      signup: Array<unknown>
    ) {
      const queue = [...signup]
      fetchMock.mockImplementation((url: string) => {
        if (new URL(url).pathname === '/legal/versions') return answer()
        if (queue.length === 0) throw new Error(`unexpected fetch: ${url}`)
        return Promise.resolve(queue.shift())
      })
    }

    const beginCalls = () =>
      fetchMock.mock.calls.filter(
        ([url]) => new URL(url as string).pathname === '/auth/passkey/signup/begin'
      )
    const versionsCalls = () =>
      fetchMock.mock.calls.filter(
        ([url]) => new URL(url as string).pathname === '/legal/versions'
      )
    const bodyOf = (call: unknown[]) =>
      JSON.parse((call[1] as { body: string }).body)

    it('begins and finishes with this build\'s versions once the route confirms them', async () => {
      fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

      renderButton(clientWithConsentVersions(BUILD_CONSENT_VERSIONS))
      await submitSignup()

      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
      for (const call of fetchMock.mock.calls) {
        expect(bodyOf(call)).toMatchObject({
          terms_version: CURRENT_TERMS_VERSION,
          privacy_version: CURRENT_PRIVACY_VERSION,
        })
      }
    })

    it('neither begins nor reports an error when the route publishes other versions', async () => {
      renderButton(clientWithConsentVersions(NEWER))
      await submitSignup()

      await waitFor(() => {
        expect(
          screen.getByRole('button', { name: /continue with passkey/i })
        ).toBeEnabled()
      })
      expect(fetchMock).not.toHaveBeenCalled()
      expect(mockOnError).not.toHaveBeenCalled()
    })

    it('neither begins again nor reports an error when the re-read after a refusal finds other versions', async () => {
      routeVersions(
        async () => publishedVersions(NEWER_VERSIONS_BODY),
        [CONSENT_REFUSAL]
      )

      renderButton(clientWithConsentVersions(BUILD_CONSENT_VERSIONS))
      await submitSignup()

      await waitFor(() => expect(versionsCalls()).toHaveLength(1))
      await waitFor(() => {
        expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      })
      expect(beginCalls()).toHaveLength(1)
      expect(mockOnError).not.toHaveBeenCalled()
      expect(mockStartRegistration).not.toHaveBeenCalled()
    })

    it('asks for a reload in the dialog, and sends nothing, while the versions changed', async () => {
      const user = userEvent.setup()
      const queryClient = clientWithConsentVersions()
      const { rerender } = renderWithProviders(<PasskeySignup />, { queryClient })
      await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
      await user.type(screen.getByLabelText('Email'), 'signup@example.com')
      await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
      await fillDateOfBirth(user)

      rerender(<PasskeySignup versionsChanged />)

      expect(screen.getByText(CONSENT_VERSIONS_CHANGED)).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: /continue with passkey/i })
      ).toBeDisabled()
      expect(fetchMock).not.toHaveBeenCalled()
    })

    it('cannot be opened while the versions changed', () => {
      renderWithProviders(<PasskeySignup versionsChanged />, {
        queryClient: clientWithConsentVersions(),
      })

      expect(
        screen.getByRole('button', { name: /sign up with passkey/i })
      ).toBeDisabled()
    })

    it('does not begin, and says why, when the versions cannot be read', async () => {
      routeVersions(async () => versionsUnavailable(), [])

      renderButton(createTestQueryClient())
      await submitSignup()

      await expectDialogError(CONSENT_VERSIONS_UNAVAILABLE)
      expect(beginCalls()).toHaveLength(0)
      expect(mockOnHandoffChange).not.toHaveBeenCalled()
    })

    it('begins again once, with a notice, after a refusal of the published versions', async () => {
      const republished = deferred<Response>()
      routeVersions(() => republished.promise, [CONSENT_REFUSAL, BEGIN_OK, FINISH_OK])

      renderButton(clientWithConsentVersions(BUILD_CONSENT_VERSIONS))
      await submitSignup()

      expect(await screen.findByText(CONSENT_RETRY_NOTICE)).toBeInTheDocument()
      expect(versionsCalls()).toHaveLength(1)
      expect((versionsCalls()[0][1] as RequestInit).cache).toBe('no-cache')
      republished.resolve(publishedVersions(BUILD_VERSIONS_BODY))

      await waitFor(() => {
        expect(mockOnHandoffChange).toHaveBeenLastCalledWith({
          status: 'complete',
          email: 'signup@example.com',
          method: 'passkey',
          step: 'backup-prompt',
        })
      })
      const [refused, retried] = beginCalls()
      expect(bodyOf(refused)).toMatchObject({ terms_version: CURRENT_TERMS_VERSION })
      expect(bodyOf(retried)).toMatchObject({
        terms_version: CURRENT_TERMS_VERSION,
        privacy_version: CURRENT_PRIVACY_VERSION,
      })
      const finish = fetchMock.mock.calls.find(
        ([url]) => new URL(url as string).pathname === '/auth/passkey/signup/finish'
      )!
      expect(bodyOf(finish)).toMatchObject({ terms_version: CURRENT_TERMS_VERSION })
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(mockOnError).not.toHaveBeenCalled()
    })

    it('reports a second refusal instead of beginning again', async () => {
      routeVersions(
        async () => publishedVersions(BUILD_VERSIONS_BODY),
        [CONSENT_REFUSAL, CONSENT_REFUSAL]
      )

      renderButton(clientWithConsentVersions(BUILD_CONSENT_VERSIONS))
      await submitSignup()

      await expectDialogError(
        'Please accept the Terms of Service and Privacy Policy before creating an account.'
      )
      expect(beginCalls()).toHaveLength(2)
      expect(versionsCalls()).toHaveLength(1)
      expect(mockStartRegistration).not.toHaveBeenCalled()
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
    })

    it('does not begin again when the versions for the retry cannot be read', async () => {
      routeVersions(
        async () => versionsUnavailable(),
        [CONSENT_REFUSAL]
      )

      renderButton(clientWithConsentVersions(BUILD_CONSENT_VERSIONS))
      await submitSignup()

      await expectDialogError(CONSENT_VERSIONS_UNAVAILABLE)
      expect(beginCalls()).toHaveLength(1)
      expect(mockStartRegistration).not.toHaveBeenCalled()
    })
  })
})
