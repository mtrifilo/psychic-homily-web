import { beforeEach, describe, expect, it, vi } from 'vitest'
import { NextRequest } from 'next/server'
import { CHART_MODULE_SLUGS } from '@/features/charts/moduleConfig'

vi.mock('@/lib/api-base', () => ({
  API_BASE_URL: 'http://backend.test',
}))

import { proxy } from './proxy'

function chartsRequest(pathname: string) {
  return new NextRequest(new URL(pathname, 'http://localhost:3000'))
}

describe('proxy charts module allowlist', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('stays in lockstep with CHART_MODULE_SLUGS', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    for (const slug of CHART_MODULE_SLUGS) {
      const response = await proxy(chartsRequest(`/charts/${slug}`))
      expect(response.status).toBe(200)
    }
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('rewrites unknown chart modules to the synthetic 404 path', async () => {
    const response = await proxy(chartsRequest('/charts/not-a-module'))

    expect(response.status).toBe(404)
    expect(response.headers.get('x-middleware-rewrite')).toContain(
      '/_psy-not-found'
    )
  })

  it('leaves the bare /charts index alone', async () => {
    const response = await proxy(chartsRequest('/charts'))
    expect(response.status).toBe(200)
  })
})
