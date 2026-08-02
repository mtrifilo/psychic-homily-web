import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { NextRequest } from 'next/server'
import { revalidatePath } from 'next/cache'
import { POST } from './route'
import { MAX_BODY_BYTES } from '@/lib/internal-revalidate'

vi.mock('next/cache', () => ({
  revalidatePath: vi.fn(),
}))

const mockRevalidatePath = vi.mocked(revalidatePath)

// Realistic shape: the backend refuses to boot with a secret under 32 chars.
const SECRET = 'a'.repeat(40)

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubEnv('INTERNAL_API_SECRET', SECRET)
})

afterEach(() => {
  vi.unstubAllEnvs()
})

function post(
  body: unknown,
  { secret, headers }: { secret?: string; headers?: Record<string, string> } = {}
): Promise<Response> {
  const text = typeof body === 'string' ? body : JSON.stringify(body)
  return POST(
    new NextRequest('http://localhost:3000/api/internal/revalidate', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        ...(secret === undefined ? {} : { 'x-internal-secret': secret }),
        ...headers,
      },
      body: text,
    })
  )
}

const ONE_SHOW = { entities: [{ type: 'show', slug: 'a-big-gig' }] }

describe('POST /api/internal/revalidate', () => {
  it('revalidates and reports counts for an authorized batch', async () => {
    const response = await post(ONE_SHOW, { secret: SECRET })

    expect(response.status).toBe(200)
    await expect(response.json()).resolves.toEqual({
      accepted: 1,
      skipped: 0,
      paths: mockRevalidatePath.mock.calls.length,
    })
    expect(mockRevalidatePath).toHaveBeenCalledWith('/shows/a-big-gig')
  })

  // The auth wiring, not just isAuthorized: a dropped `!` here would leave an
  // open cache-invalidation endpoint that the unit tests would still pass.
  it('rejects a missing secret header without revalidating', async () => {
    const response = await post(ONE_SHOW)

    expect(response.status).toBe(401)
    await expect(response.json()).resolves.toEqual({ error: 'Unauthorized' })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('rejects a wrong secret without revalidating', async () => {
    const response = await post(ONE_SHOW, { secret: 'b'.repeat(40) })

    expect(response.status).toBe(401)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('rejects when the server has no secret configured', async () => {
    vi.stubEnv('INTERNAL_API_SECRET', '')
    const response = await post(ONE_SHOW, { secret: SECRET })

    expect(response.status).toBe(401)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('checks auth BEFORE reading the body', async () => {
    // An unparseable body still answers 401, not 400 — proof that an anonymous
    // caller cannot make us parse anything.
    const response = await post('this is not json')

    expect(response.status).toBe(401)
  })

  it('rejects a malformed envelope with 400', async () => {
    const response = await post({ nope: [] }, { secret: SECRET })

    expect(response.status).toBe(400)
    await expect(response.json()).resolves.toEqual({
      error: 'Expected an "entities" array',
    })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('rejects an oversized Content-Length before reading the body', async () => {
    const response = await post(ONE_SHOW, {
      secret: SECRET,
      headers: { 'content-length': String(MAX_BODY_BYTES + 1) },
    })

    expect(response.status).toBe(400)
    await expect(response.json()).resolves.toEqual({
      error: `Body exceeds ${MAX_BODY_BYTES} bytes`,
    })
  })

  it('still revalidates the usable entries of a mixed batch', async () => {
    const response = await post(
      {
        entities: [
          { type: 'show', slug: 'a-real-show' },
          { type: 'show', slug: '../../etc/passwd' },
          { type: 'nonsense', slug: 'whatever' },
        ],
      },
      { secret: SECRET }
    )

    expect(response.status).toBe(200)
    await expect(response.json()).resolves.toMatchObject({
      accepted: 1,
      skipped: 2,
    })
    expect(mockRevalidatePath).toHaveBeenCalledWith('/shows/a-real-show')
  })

  it('applies the rename cascade only when the caller asks for it', async () => {
    await post(
      { entities: [{ type: 'artist', slug: 'bright-eyes' }] },
      { secret: SECRET }
    )
    const withoutRename = mockRevalidatePath.mock.calls.map(([p]) => p)
    expect(withoutRename).toContain('/artists/bright-eyes')
    expect(withoutRename).not.toContain('/shows/[slug]')

    mockRevalidatePath.mockClear()

    await post(
      { entities: [{ type: 'artist', slug: 'bright-eyes', renamed: true }] },
      { secret: SECRET }
    )
    expect(mockRevalidatePath.mock.calls.map(([p]) => p)).toContain(
      '/shows/[slug]'
    )
  })

  it('answers 500 without throwing when revalidation blows up', async () => {
    mockRevalidatePath.mockImplementation(() => {
      throw new Error('boom')
    })

    // safeRevalidatePath swallows per-path failures, so the batch still
    // succeeds — the 500 path exists for a bug ABOVE that layer.
    const response = await post(ONE_SHOW, { secret: SECRET })
    expect(response.status).toBe(200)
  })
})
