import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { createTestQueryClient, renderWithProviders } from '@/test/utils'
import {
  clientWithConsentVersions,
  publishedVersions,
  versionsUnavailable,
} from '@/test/consentVersions'
import { deferred } from '@/test/inflightOrdering'
import {
  CONSENT_RETRY_NOTICE,
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

const mockSetUser = vi.fn()
const mockOnHandoffChange = vi.fn()
const mockOnDeadlineExpire = vi.fn()
const mockOnError = vi.fn()
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
    mockOnHandoffChange.mockReset()
    mockOnDeadlineExpire.mockReset()
    mockOnError.mockReset()
    mockStartRegistration.mockReset()
    fetchMock.mockReset()

    mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // Stands in for the page, which owns the deadline in production.
  function PasskeySignup() {
    const requestDeadline = useSignupRequestDeadline(mockOnDeadlineExpire)
    return (
      <PasskeySignupButton
        onHandoffChange={mockOnHandoffChange}
        onError={mockOnError}
        requestDeadline={requestDeadline}
      />
    )
  }

  // Seeded by default, so the fetch calls a test counts are the sign-up's own.
  function renderButton(queryClient = clientWithConsentVersions()) {
    renderWithProviders(<PasskeySignup />, { queryClient })
  }

  async function submitSignup() {
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    // Two required checkboxes: terms + age confirmation.
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
    await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
    await user.click(
      screen.getByRole('button', { name: /continue with passkey/i })
    )
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
      terms_version: '2026-01-31',
      privacy_version: '2026-02-15',
      age_confirmed: true,
      min_age_attested: 16,
    })

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
    expect(finishSignupPayload).toMatchObject({
      age_confirmed: true,
      min_age_attested: 16,
    })

    await waitFor(() => {
      expect(mockOnHandoffChange).toHaveBeenCalledTimes(2)
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [{ status: 'complete', email: 'signup@example.com', method: 'passkey' }],
    ])
    expect(mockSetUser).toHaveBeenCalledWith(
      expect.objectContaining({ id: 100, email: 'signup@example.com' })
    )
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

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Failed to complete passkey signup')
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
  })

  it('releases its claim and reports the error when finish is refused', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Email already registered')
    })
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

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Service temporarily unavailable')
    })
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

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Email already registered')
    })
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

  describe('under the request deadline', () => {
    // fireEvent rather than userEvent: no focus moves, so nothing but this
    // component schedules work while the fake clock runs.
    function submitWithFireEvent() {
      fireEvent.click(screen.getByRole('button', { name: /sign up with passkey/i }))
      fireEvent.change(screen.getByLabelText('Email'), {
        target: { value: 'signup@example.com' },
      })
      fireEvent.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
      fireEvent.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
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
        await waitFor(() => {
          expect(mockOnError).toHaveBeenCalledWith('Failed to complete passkey signup')
        })
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
      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith('Failed to start passkey signup')
      })
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
      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith('Email already registered')
      })

      await advanceFakeTime(SIGNUP_REQUEST_TIMEOUT_MS * 2)
      expect(requestSignal(fetchMock, 1).aborted).toBe(false)
      expect(mockOnDeadlineExpire).not.toHaveBeenCalled()
      expect(mockOnError).toHaveBeenCalledTimes(1)
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
        [{ status: 'complete', email: 'signup@example.com', method: 'passkey' }],
      ])
      expect(mockOnError).not.toHaveBeenCalled()
      expect(mockOnDeadlineExpire).not.toHaveBeenCalled()
    })
  })

  // PSY-1023: the Continue button is disabled and no signup request fires until
  // the age confirmation is checked, even with email + terms provided.
  it('blocks passkey signup until age is confirmed', async () => {
    const user = userEvent.setup()

    renderButton()

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))

    // Age unchecked → Continue button stays disabled and no fetch fires.
    expect(
      screen.getByRole('button', { name: /continue with passkey/i })
    ).toBeDisabled()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  // Begin sends the versions GET /legal/versions publishes, never the build
  // constants, and a consent refusal of them is retried once with versions
  // read again past every cache.
  describe('consent versions', () => {
    const PUBLISHED = { termsVersion: '2031-03-01', privacyVersion: '2031-03-02' }
    const REPUBLISHED = { terms_version: '2031-04-01', privacy_version: '2031-04-02' }
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

    it('begins and finishes with the versions the route publishes', async () => {
      fetchMock.mockResolvedValueOnce(BEGIN_OK).mockResolvedValueOnce(FINISH_OK)

      renderButton(clientWithConsentVersions(PUBLISHED))
      await submitSignup()

      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
      for (const call of fetchMock.mock.calls) {
        expect(bodyOf(call)).toMatchObject({
          terms_version: '2031-03-01',
          privacy_version: '2031-03-02',
        })
      }
    })

    it('does not begin, and says why, when the versions cannot be read', async () => {
      routeVersions(async () => versionsUnavailable(), [])

      renderButton(createTestQueryClient())
      await submitSignup()

      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith(CONSENT_VERSIONS_UNAVAILABLE)
      })
      expect(beginCalls()).toHaveLength(0)
      expect(mockOnHandoffChange).not.toHaveBeenCalled()
    })

    it('begins again once, with a notice, after a refusal of the published versions', async () => {
      const republished = deferred<Response>()
      routeVersions(() => republished.promise, [CONSENT_REFUSAL, BEGIN_OK, FINISH_OK])

      renderButton(clientWithConsentVersions(PUBLISHED))
      await submitSignup()

      expect(await screen.findByText(CONSENT_RETRY_NOTICE)).toBeInTheDocument()
      expect(versionsCalls()).toHaveLength(1)
      expect((versionsCalls()[0][1] as RequestInit).cache).toBe('no-cache')
      republished.resolve(publishedVersions(REPUBLISHED))

      await waitFor(() => {
        expect(mockOnHandoffChange).toHaveBeenLastCalledWith({
          status: 'complete',
          email: 'signup@example.com',
          method: 'passkey',
        })
      })
      const [refused, retried] = beginCalls()
      expect(bodyOf(refused)).toMatchObject({ terms_version: '2031-03-01' })
      expect(bodyOf(retried)).toMatchObject({
        terms_version: '2031-04-01',
        privacy_version: '2031-04-02',
      })
      const finish = fetchMock.mock.calls.find(
        ([url]) => new URL(url as string).pathname === '/auth/passkey/signup/finish'
      )!
      expect(bodyOf(finish)).toMatchObject({ terms_version: '2031-04-01' })
      expect(screen.queryByText(CONSENT_RETRY_NOTICE)).not.toBeInTheDocument()
      expect(mockOnError).not.toHaveBeenCalled()
    })

    it('reports a second refusal instead of beginning again', async () => {
      routeVersions(
        async () => publishedVersions(REPUBLISHED),
        [CONSENT_REFUSAL, CONSENT_REFUSAL]
      )

      renderButton(clientWithConsentVersions(PUBLISHED))
      await submitSignup()

      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith(
          'Please accept the Terms of Service and Privacy Policy before creating an account.'
        )
      })
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

      renderButton(clientWithConsentVersions(PUBLISHED))
      await submitSignup()

      await waitFor(() => {
        expect(mockOnError).toHaveBeenCalledWith(CONSENT_VERSIONS_UNAVAILABLE)
      })
      expect(beginCalls()).toHaveLength(1)
      expect(mockStartRegistration).not.toHaveBeenCalled()
    })
  })
})
