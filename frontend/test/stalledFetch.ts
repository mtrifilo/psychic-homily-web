import { act } from '@testing-library/react'
import { vi, type Mock } from 'vitest'

/**
 * Fetch doubles for a request on a stalled connection. Nothing settles until
 * the request's signal aborts, and then the pending promise rejects with an
 * AbortError, the way fetch does for both the response and its body.
 */

function untilAborted(signal: AbortSignal | null | undefined): Promise<never> {
  return new Promise((_, reject) => {
    signal?.addEventListener('abort', () => {
      reject(new DOMException('The operation was aborted.', 'AbortError'))
    })
  })
}

/** A fetch whose response never arrives. */
export function stalledRequest(_url: string, init?: RequestInit): Promise<never> {
  return untilAborted(init?.signal)
}

/** A 200 whose headers have arrived; reading its JSON body calls `body`. */
export function headersThenBody(body: () => Promise<unknown>): Response {
  return {
    ok: true,
    status: 200,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: body,
  } as unknown as Response
}

/** A fetch whose 200 headers arrive and whose JSON body never does. */
export async function stalledResponseBody(_url: string, init?: RequestInit) {
  return headersThenBody(() => untilAborted(init?.signal))
}

/** The signal handed to the fetch mock's `call`th request. */
export function requestSignal(fetchMock: Mock, call: number): AbortSignal {
  return (fetchMock.mock.calls[call]?.[1] as RequestInit).signal as AbortSignal
}

/** Advances the fake clock inside act, flushing the work each timer starts. */
export async function advanceFakeTime(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}
