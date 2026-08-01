import { describe, it, expect, vi, afterEach } from 'vitest'
import { loadRemoteImage } from './remoteImage'

/** The image when the load succeeded, else null — most assertions only care. */
async function loaded(url: string | null | undefined) {
  const outcome = await loadRemoteImage(url)
  return outcome.status === 'ok' ? outcome.image : null
}

/** The outcome tag, which is what decides how long a card may be cached. */
async function outcomeOf(url: string | null | undefined) {
  return (await loadRemoteImage(url)).status
}

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
  it('returns the raw bytes and the intrinsic size for a real image', async () => {
    mockFetch(() => imageResponse(pngBytes(800, 1000)))
    const result = await loaded('https://cdn.example.com/flyer.png')
    expect(result?.width).toBe(800)
    expect(result?.height).toBe(1000)
    // An ArrayBuffer, not a view and not a data URI: Satori sizes it with
    // `new DataView(e)`, which rejects a typed array, and its data-URI branch
    // retains every URI in a module-scoped LRU.
    expect(result?.data).toBeInstanceOf(ArrayBuffer)
    expect(new Uint8Array(result!.data)).toEqual(pngBytes(800, 1000))
  })

  // The guard that keeps the route fail-open: Satori draws png/apng/jpeg/gif
  // and THROWS on anything else, so a WebP flyer must be dropped here rather
  // than handed on.
  it('drops a WebP the rasteriser cannot draw', async () => {
    const webp = new Uint8Array(30)
    webp.set([...'RIFF'].map(c => c.charCodeAt(0)), 0)
    webp.set([...'WEBP'].map(c => c.charCodeAt(0)), 8)
    webp.set([...'VP8X'].map(c => c.charCodeAt(0)), 12)
    mockFetch(() => imageResponse(webp))
    expect(await loaded('https://cdn.example.com/flyer.webp')).toBeNull()
  })

  it('requests on guarded terms: no ambient auth, manual redirects, a deadline', async () => {
    const spy = mockFetch(() => imageResponse(pngBytes()))
    await loaded('https://cdn.example.com/flyer.png')
    expect(spy.mock.calls[0][1]).toMatchObject({
      credentials: 'omit',
      referrerPolicy: 'no-referrer',
      // Manual is what makes the per-hop host checks apply at all.
      redirect: 'manual',
    })
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

    // IPv4-mapped IPv6. These defeated a prefix-matching guard completely: the
    // URL parser canonicalises `[::ffff:169.254.169.254]` to `[::ffff:a9fe:a9fe]`,
    // so no dotted-quad check can see it, and on a dual-stack host it connects
    // to the IPv4 address. Every literal-IP defence was expressible this way.
    ['IPv4-mapped metadata endpoint', 'https://[::ffff:169.254.169.254]/latest/meta-data/'],
    ['IPv4-mapped metadata, hex form', 'https://[::ffff:a9fe:a9fe]/latest/meta-data/'],
    ['IPv4-mapped loopback', 'https://[::ffff:127.0.0.1]/admin'],
    ['IPv4-mapped RFC1918', 'https://[::ffff:10.0.0.1]/'],
    ['fully expanded IPv4-mapped', 'https://[0:0:0:0:0:ffff:a9fe:a9fe]/'],
    ['the unspecified address', 'https://[::]/admin'],
    ['IPv6 unique-local', 'https://[fd00::1]/'],
    ['IPv6 link-local', 'https://[fe80::1]/'],
    ['NAT64', 'https://[64:ff9b::a9fe:a9fe]/'],

    // A trailing dot is a valid absolute FQDN that resolvers honour, and it
    // slipped every name-based check by adding one character.
    ['loopback with a trailing dot', 'https://localhost./flyer.png'],
    ['metadata with a trailing dot', 'https://metadata.google.internal./v1/'],
    ['internal suffix with a trailing dot', 'https://backend.railway.internal./health'],

    // Ranges a five-entry denylist misses.
    ['CGNAT 100.64/10', 'https://100.64.1.1/flyer.png'],
    ['Alibaba metadata', 'https://100.100.100.200/latest/meta-data/'],
    ['Oracle metadata 192.0.0/24', 'https://192.0.0.192/opc/v1/instance/'],
    ['this-network 0/8', 'https://0.0.0.0/flyer.png'],
    ['multicast', 'https://224.0.0.1/flyer.png'],
    ['benchmarking 198.18/15', 'https://198.18.0.1/flyer.png'],
  ] as const

  for (const [label, url] of blocked) {
    it(`makes no request for ${label}`, async () => {
      const spy = mockFetch(() => imageResponse(pngBytes()))
      expect(await loaded(url)).toBeNull()
      expect(spy).not.toHaveBeenCalled()
    })
  }

  it('makes no request for an absent flyer', async () => {
    const spy = mockFetch(() => imageResponse(pngBytes()))
    expect(await loaded(null)).toBeNull()
    expect(await loaded(undefined)).toBeNull()
    expect(await loaded('')).toBeNull()
    expect(spy).not.toHaveBeenCalled()
  })

  // 172.15 and 172.32 are PUBLIC — the private range is 172.16–172.31, and an
  // over-broad regex here would silently break real hosts.
  it('does not over-block addresses just outside RFC1918', async () => {
    mockFetch(() => imageResponse(pngBytes()))
    expect(await loaded('https://172.15.0.1/flyer.png')).not.toBeNull()
    expect(await loaded('https://172.32.0.1/flyer.png')).not.toBeNull()
    expect(await loaded('https://11.0.0.1/flyer.png')).not.toBeNull()
    expect(await loaded('https://100.63.0.1/flyer.png')).not.toBeNull() // just below CGNAT
    expect(await loaded('https://100.128.0.1/flyer.png')).not.toBeNull() // just above
  })

  // A real CDN on IPv6 has to keep working — the rule is "global unicast only",
  // not "no IPv6".
  it('allows a global-unicast IPv6 host', async () => {
    mockFetch(() => imageResponse(pngBytes()))
    expect(await loaded('https://[2606:4700::6810:85e5]/flyer.png')).not.toBeNull()
  })

  it('allows an ordinary hostname that merely contains a blocked word', async () => {
    mockFetch(() => imageResponse(pngBytes()))
    expect(await loaded('https://localhost.example.com/flyer.png')).not.toBeNull()
    expect(await loaded('https://internal.example.com/flyer.png')).not.toBeNull()
  })
})

describe('the outcome tag that decides caching', () => {
  // A rejected flyer is a permanent fact about the row — an `http:` URL the
  // backend accepts but this does not, a WebP, an oversized scan. Caching those
  // cards for 60s would re-render them forever.
  it('reports a permanent rejection as `rejected`', async () => {
    mockFetch(() => imageResponse(pngBytes()))
    expect(await outcomeOf('http://cdn.example.com/flyer.png')).toBe('rejected')
    expect(await outcomeOf('https://169.254.169.254/x')).toBe('rejected')

    mockFetch(() => imageResponse(pngBytes(4000, 4000)))
    expect(await outcomeOf('https://cdn.example.com/huge.png')).toBe('rejected')

    mockFetch(() => new Response('nope', { status: 404 }))
    expect(await outcomeOf('https://cdn.example.com/gone.png')).toBe('rejected')
  })

  // A 5xx or a timeout could succeed on the next unfurl, so those DO earn the
  // short cache window.
  it('reports a transient failure as `unavailable`', async () => {
    mockFetch(() => new Response('oops', { status: 503 }))
    expect(await outcomeOf('https://cdn.example.com/flyer.png')).toBe('unavailable')

    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new DOMException('The operation was aborted', 'TimeoutError')
    }))
    expect(await outcomeOf('https://cdn.example.com/slow.png')).toBe('unavailable')
  })

  it('reports no flyer at all as `none`', async () => {
    expect(await outcomeOf(null)).toBe('none')
    expect(await outcomeOf('')).toBe('none')
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

    expect(await loaded('https://cdn.example.com/flyer.png')).toBeNull()
    expect(seen).toEqual(['https://cdn.example.com/flyer.png'])
  })

  it('refuses a redirect that downgrades to http', async () => {
    mockFetch(() =>
      new Response(null, {
        status: 301,
        headers: { location: 'http://cdn.example.com/flyer.png' },
      })
    )
    expect(await loaded('https://cdn.example.com/flyer.png')).toBeNull()
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
    const result = await loaded('https://cdn.example.com/flyer.png')
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
    expect(await loaded('https://cdn.example.com/flyer.png')).not.toBeNull()
    expect(seen[1]).toBe('https://cdn.example.com/real/flyer.png')
  })

  it('gives up on a redirect loop rather than chasing it forever', async () => {
    const spy = mockFetch(url =>
      new Response(null, { status: 302, headers: { location: `${url.href}?again` } })
    )
    expect(await loaded('https://cdn.example.com/flyer.png')).toBeNull()
    expect(spy.mock.calls.length).toBeLessThanOrEqual(4)
  })

  it('refuses a redirect with no readable Location', async () => {
    mockFetch(() => new Response(null, { status: 302 }))
    expect(await loaded('https://cdn.example.com/flyer.png')).toBeNull()
  })
})

describe('responses that must not reach the renderer', () => {
  it('drops a non-image body', async () => {
    mockFetch(() => imageResponse(new TextEncoder().encode('<html>internal admin</html>')))
    expect(await loaded('https://cdn.example.com/flyer.png')).toBeNull()
  })

  it('drops an error status', async () => {
    for (const status of [401, 403, 404, 500]) {
      mockFetch(() => new Response('nope', { status }))
      expect(await loaded('https://cdn.example.com/flyer.png'), String(status)).toBeNull()
    }
  })

  // A byte cap cannot express decode cost: a small JPEG can declare an enormous
  // canvas, and rasterising it at 4 bytes a pixel is how the runtime runs out
  // of memory.
  it('drops an image whose declared canvas exceeds the pixel budget', async () => {
    mockFetch(() => imageResponse(pngBytes(12000, 12000)))
    expect(await loaded('https://cdn.example.com/huge.png')).toBeNull()
  })

  // The byte cap is a ceiling, not a target: sized to the 380×510 plate it
  // threw away a real 1.88MB Commons poster at 1.5MB.
  it('admits the hosted flyers the feature is actually for', async () => {
    for (const [w, h] of [
      [960, 1387], // the measured Commons poster, 1.33MP
      [1080, 1350], // Instagram portrait export, 1.46MP
      [1600, 2400], // a generous hosted flyer, 3.84MP
    ] as const) {
      mockFetch(() => imageResponse(pngBytes(w, h)))
      expect(await loaded('https://cdn.example.com/flyer.png'), `${w}×${h}`).not.toBeNull()
    }
  })

  // Print-resolution and camera-original sources are ABOVE the cap by design,
  // and this is the measured reason rather than a guess: rendering an 8.41MP
  // PNG through this bundle costs ~236MB of peak RSS over baseline, against a
  // 128MB edge budget. An OOM is a platform kill that goes past every
  // fail-open path, so these degrade to the text card instead.
  it('drops sources whose decode would not fit the runtime', async () => {
    for (const [w, h] of [
      [2550, 3300], // 8.5×11 at 300dpi — 8.41MP, measured at ~+236MB
      [4032, 3024], // untouched phone photo — 12.19MP
      [12000, 12000], // the decompression bomb the cap exists for
    ] as const) {
      mockFetch(() => imageResponse(pngBytes(w, h)))
      expect(await loaded('https://cdn.example.com/big.png'), `${w}×${h}`).toBeNull()
    }
  })

  it('drops a zero-dimension image', async () => {
    mockFetch(() => imageResponse(pngBytes(0, 0)))
    expect(await loaded('https://cdn.example.com/empty.png')).toBeNull()
  })

  it('drops a response that declares itself over the byte cap', async () => {
    mockFetch(() =>
      new Response(body(pngBytes()), {
        status: 200,
        headers: { 'content-length': String(50 * 1024 * 1024) },
      })
    )
    expect(await loaded('https://cdn.example.com/big.png')).toBeNull()
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
    expect(await loaded('https://cdn.example.com/endless.png')).toBeNull()
  })

  // Every failure is the caller's cue to render its text-only card, so none of
  // them may escape as an exception.
  it('returns null rather than throwing when the fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('network unreachable')
    }))
    await expect(loaded('https://cdn.example.com/flyer.png')).resolves.toBeNull()
  })

  it('returns null rather than throwing when the fetch times out', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new DOMException('The operation was aborted', 'TimeoutError')
    }))
    await expect(loaded('https://cdn.example.com/slow.png')).resolves.toBeNull()
  })
})
