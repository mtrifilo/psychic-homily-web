import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { proxy } from './proxy'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

describe('proxy entity existence checks', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the lightweight HEAD exists probe instead of the full detail GET', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/shows/e2e-attendance-test'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/entities/shows/e2e-attendance-test/exists',
      {
        method: 'HEAD',
        redirect: 'manual',
      }
    )
  })

  it('rewrites backend 404 probes to a real not-found response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 404 })
    )

    const response = await proxy(requestFor('/tags/missing-tag'))

    expect(response.status).toBe(404)
  })

  it('does not existence-check reserved static show routes', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    await proxy(requestFor('/shows/submit'))
    await proxy(requestFor('/shows/saved'))

    expect(fetchMock).not.toHaveBeenCalled()
  })
})
