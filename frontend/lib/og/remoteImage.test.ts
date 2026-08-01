import { describe, it, expect, vi, afterEach } from 'vitest'
import { loadRemoteImage } from './remoteImage'

/**
 * These are the guards on an attacker-controlled fetch.
 *
 * `shows.image_url` is writable by any email-verified user and the backend
 * validates only the scheme and a 2048-char cap, so every value reaching
 * `loadRemoteImage` should be read as hostile. The assertions below are mostly
 * about what is NOT requested.
 */

function pngBytes(width = 100, height = 120): Uint8Array {
  const b = new Uint8Array(24)
  b.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a], 0)
  b.set([0x49, 0x48, 0x44, 0x52], 12)
  const view = new DataView(b.buffer)
  view.setUint32(16, width)
  view.setUint32(20, height)
  return b
}

function body(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
}

function imageResponse(bytes: Uint8Array, init: ResponseInit = {}): Response {
  return new Response(body(bytes), {
    status: 200,
    headers: { 'content-length': String(bytes.byteLength) },
    ...init,
  })
}

function mockFetch(impl: (url: URL, init?: RequestInit) => Response | Promise<Response>) {
  const spy = vi.fn(async (input: string | URL | Request, init?: RequestInit) =>
    impl(new URL(String(input)), init)
  )
  vi.stubGlobal('fetch', spy)
  return spy
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('loadRemoteImage', () => {
  it('returns a data URI and the intrinsic size for a real image', async () => {
    mockFetch(() => imageResponse(pngBytes(800, 1000)))
    const result = await loadRemoteImage('https://cdn.example.com/flyer.png')
    expect(result?.width).toBe(800)
    expect(result?.height).toBe(1000)
    expect(result?.dataUri.startsWith('data:image/png;base64,')).toBe(true)
  })

  it('sends no ambient credentials or referrer', async () => {
    const spy = mockFetch(() => imageResponse(pngBytes()))
    await loadRemoteImage('https://cdn.example.com/flyer.png')
    expect(spy.mock.calls[0][1]).toMatchObject({
      credentials: 'omit',
      referrerPolicy: 'no-referrer',
    })
  })

  it('bounds the fetch with a deadline', async () => {
    const spy = mockFetch(() => imageResponse(pngBytes()))
    await loadRemoteImage('https://cdn.example.com/flyer.png')
    expect(spy.mock.calls[0][1]?.signal).toBeInstanceOf(AbortSignal)
  })
})

describe('URLs that must never be fetched at all', () => {
  const blocked = [
    ['http, not https', 'http://cdn.example.com/flyer.png'],
    ['loopback by name', 'https://localhost/flyer.png'],
    ['loopback by address', 'https://127.0.0.1/flyer.png'],
    ['IPv6 loopback', 'https://[::1]/flyer.png'],
    ['the cloud metadata endpoint', 'https://169.254.169.254/latest/meta-data/'],
    ['metadata by name', 'https://metadata.google.internal/computeMetadata/v1/'],
    ['RFC1918 10/8', 'https://10.0.0.5/flyer.png'],
    ['RFC1918 192.168/16', 'https://192.168.1.1/flyer.png'],
    ['RFC1918 172.16/12', 'https://172.20.0.1/flyer.png'],
    ['an internal service name', 'https://backend.railway.internal/health'],
    ['a .local name', 'https://printer.local/flyer.png'],
    ['a file URL', 'file:///etc/passwd'],
    ['a data URL', 'data:image/png;base64,AAAA'],
    ['a javascript URL', 'javascript:alert(1)'],
    ['unparseable junk', 'not a url'],
  ] as const

  for (const [label, url] of blocked) {
    it(`makes no request for ${label}`, async () => {
      const spy = mockFetch(() => imageResponse(pngBytes()))
      expect(await loadRemoteImage(url)).toBeNull()
      expect(spy).not.toHaveBeenCalled()
    })
  }

  it('makes no request for an absent flyer', async () => {
    const spy = mockFetch(() => imageResponse(pngBytes()))
    expect(await loadRemoteImage(null)).toBeNull()
    expect(await loadRemoteImage(undefined)).toBeNull()
    expect(await loadRemoteImage('')).toBeNull()
    expect(spy).not.toHaveBeenCalled()
  })

  // 172.15 and 172.32 are PUBLIC — the private range is 172.16–172.31, and an
  // over-broad regex here would silently break real hosts.
  it('does not over-block addresses just outside RFC1918', async () => {
    mockFetch(() => imageResponse(pngBytes()))
    expect(await loadRemoteImage('https://172.15.0.1/flyer.png')).not.toBeNull()
    expect(await loadRemoteImage('https://172.32.0.1/flyer.png')).not.toBeNull()
    expect(await loadRemoteImage('https://11.0.0.1/flyer.png')).not.toBeNull()
  })
})

describe('the redirect chain', () => {
  // The reason redirects are followed by hand: `redirect: 'follow'` would have
  // validated only the first URL, so a public host could bounce the request
  // straight into the metadata service.
  it('refuses a redirect into a blocked host', async () => {
    const seen: string[] = []
    mockFetch(url => {
      seen.push(url.href)
      if (url.hostname === 'cdn.example.com') {
        return new Response(null, {
          status: 302,
          headers: { location: 'https://169.254.169.254/latest/meta-data/' },
        })
      }
      return imageResponse(pngBytes())
    })

    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).toBeNull()
    expect(seen).toEqual(['https://cdn.example.com/flyer.png'])
  })

  it('refuses a redirect that downgrades to http', async () => {
    mockFetch(() =>
      new Response(null, {
        status: 301,
        headers: { location: 'http://cdn.example.com/flyer.png' },
      })
    )
    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).toBeNull()
  })

  it('follows a redirect to another allowed host', async () => {
    mockFetch(url =>
      url.hostname === 'cdn.example.com'
        ? new Response(null, {
            status: 302,
            headers: { location: 'https://images.example.net/real.png' },
          })
        : imageResponse(pngBytes(640, 800))
    )
    const result = await loadRemoteImage('https://cdn.example.com/flyer.png')
    expect(result?.width).toBe(640)
  })

  it('resolves a relative Location against the current hop', async () => {
    const seen: string[] = []
    mockFetch(url => {
      seen.push(url.href)
      if (url.pathname === '/flyer.png') {
        return new Response(null, { status: 302, headers: { location: '/real/flyer.png' } })
      }
      return imageResponse(pngBytes())
    })
    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).not.toBeNull()
    expect(seen[1]).toBe('https://cdn.example.com/real/flyer.png')
  })

  it('gives up on a redirect loop rather than chasing it forever', async () => {
    const spy = mockFetch(url =>
      new Response(null, { status: 302, headers: { location: `${url.href}?again` } })
    )
    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).toBeNull()
    expect(spy.mock.calls.length).toBeLessThanOrEqual(4)
  })

  it('refuses a redirect with no readable Location', async () => {
    mockFetch(() => new Response(null, { status: 302 }))
    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).toBeNull()
  })
})

describe('responses that must not reach the renderer', () => {
  it('drops a non-image body', async () => {
    mockFetch(() => imageResponse(new TextEncoder().encode('<html>internal admin</html>')))
    expect(await loadRemoteImage('https://cdn.example.com/flyer.png')).toBeNull()
  })

  it('drops an error status', async () => {
    for (const status of [401, 403, 404, 500]) {
      mockFetch(() => new Response('nope', { status }))
      expect(await loadRemoteImage('https://cdn.example.com/flyer.png'), String(status)).toBeNull()
    }
  })

  // A byte cap cannot express decode cost: a small JPEG can declare an enormous
  // canvas, and rasterising it at 4 bytes a pixel is how the runtime runs out
  // of memory.
  it('drops an image whose declared canvas exceeds the pixel budget', async () => {
    mockFetch(() => imageResponse(pngBytes(12000, 12000)))
    expect(await loadRemoteImage('https://cdn.example.com/huge.png')).toBeNull()
  })

  it('drops a zero-dimension image', async () => {
    mockFetch(() => imageResponse(pngBytes(0, 0)))
    expect(await loadRemoteImage('https://cdn.example.com/empty.png')).toBeNull()
  })

  it('drops a response that declares itself over the byte cap', async () => {
    mockFetch(() =>
      new Response(body(pngBytes()), {
        status: 200,
        headers: { 'content-length': String(50 * 1024 * 1024) },
      })
    )
    expect(await loadRemoteImage('https://cdn.example.com/big.png')).toBeNull()
  })

  // The cap that matters: a chunked response declares no length at all, so it
  // has to be enforced while streaming or not at all.
  it('drops an undeclared body that runs past the byte cap', async () => {
    mockFetch(
      () =>
        new Response(
          new ReadableStream({
            pull(controller) {
              controller.enqueue(new Uint8Array(512 * 1024))
            },
          }),
          { status: 200 }
        )
    )
    expect(await loadRemoteImage('https://cdn.example.com/endless.png')).toBeNull()
  })

  // Every failure is the caller's cue to render its text-only card, so none of
  // them may escape as an exception.
  it('returns null rather than throwing when the fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('network unreachable')
    }))
    await expect(loadRemoteImage('https://cdn.example.com/flyer.png')).resolves.toBeNull()
  })

  it('returns null rather than throwing when the fetch times out', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new DOMException('The operation was aborted', 'TimeoutError')
    }))
    await expect(loadRemoteImage('https://cdn.example.com/slow.png')).resolves.toBeNull()
  })
})
