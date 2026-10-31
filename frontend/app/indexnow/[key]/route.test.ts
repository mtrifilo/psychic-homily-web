import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('next/server', async importOriginal => ({
  ...(await importOriginal<typeof import('next/server')>()),
  connection: vi.fn(async () => {}),
}))

import { GET } from './route'

const KEY = 'abcdef0123456789'

function get(key: string): Promise<Response> {
  return GET(new Request(`https://psychichomily.com/indexnow/${key}`), {
    params: Promise.resolve({ key }),
  })
}

describe('GET /indexnow/[key]', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('answers the key as text/plain for an exact match', async () => {
    vi.stubEnv('INDEXNOW_KEY', KEY)
    const res = await get(KEY)
    expect(res.status).toBe(200)
    expect(res.headers.get('Content-Type')).toBe('text/plain; charset=utf-8')
    await expect(res.text()).resolves.toBe(KEY)
  })

  it.each([
    ['a different key', 'fedcba9876543210'],
    ['a prefix of the key', KEY.slice(0, -1)],
    ['the key with a suffix', `${KEY}0`],
    ['the key in another case', KEY.toUpperCase()],
    ['an empty key', ''],
  ])('404s %s', async (_label, requested) => {
    vi.stubEnv('INDEXNOW_KEY', KEY)
    const res = await get(requested)
    expect(res.status).toBe(404)
    await expect(res.text()).resolves.not.toContain(KEY)
  })

  it.each([
    ['unset', undefined],
    ['empty', ''],
    ['blank', '   '],
  ])('404s every key while INDEXNOW_KEY is %s', async (_label, value) => {
    vi.stubEnv('INDEXNOW_KEY', value)
    expect((await get(KEY)).status).toBe(404)
    expect((await get('')).status).toBe(404)
  })
})
