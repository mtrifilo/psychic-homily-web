/**
 * Proves whether rendering a given string reaches a third party.
 *
 * Not a unit test: it drives the real `next/og` `ImageResponse` with the real
 * shipped `.ttf` bytes and records every outbound request the render makes, so
 * it answers the actual acceptance question ("does this card render without a
 * third-party request?") rather than a proxy for it.
 *
 *     bun run scripts/og-font-egress-probe.tsx [--emit-png <dir>]
 *
 * Exits non-zero if a string the subset claims to cover still hits the network,
 * or if a string documented as UNCOVERED silently stops hitting it (which would
 * mean the residual-gap section of `lib/og/brand.ts` has gone stale).
 */
import { ImageResponse } from 'next/og'
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const FONTS = 'lib/og/fonts'
const read = (f: string) => {
  const b = readFileSync(join(FONTS, f))
  return b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength) as ArrayBuffer
}

// Mirrors fetchBrandFonts() in lib/og/brand.ts exactly -- same files, same
// family names, same weights, same order. Rebuilt here rather than imported
// because the module loads its assets through `fetch(new URL(..., import.meta.url))`,
// which is a bundler affordance that does not resolve outside a Next build.
const brandOnly = process.argv.includes('--no-fallback')

const fonts = [
  { name: 'Satoshi', data: read('Satoshi-Bold.ttf'), weight: 700 as const, style: 'normal' as const },
  { name: 'Satoshi', data: read('Satoshi-Medium.ttf'), weight: 500 as const, style: 'normal' as const },
  { name: 'Satoshi', data: read('Satoshi-Regular.ttf'), weight: 400 as const, style: 'normal' as const },
  { name: 'Space Mono', data: read('SpaceMono-Regular.ttf'), weight: 400 as const, style: 'normal' as const },
  // `--no-fallback` reproduces the pre-PSY-1640 font set, so the covered cases
  // below can be shown FAILING without it. A probe that only ever passes proves
  // nothing about whether the added faces are what is doing the work.
  ...(brandOnly
    ? []
    : [
        { name: 'PH Fallback', data: read('NotoSans-Regular.ttf'), weight: 400 as const, style: 'normal' as const },
        { name: 'PH Fallback', data: read('NotoSans-Bold.ttf'), weight: 700 as const, style: 'normal' as const },
      ]),
]

const THIRD_PARTY = ['fonts.googleapis.com', 'fonts.gstatic.com', 'cdn.jsdelivr.net']

/** Cases the widened subset promises to render offline. */
const COVERED: Array<[string, string]> = [
  ['Cyrillic (Russian)', 'Москва'],
  ['Cyrillic (Ukrainian)', 'Київ'],
  ['Cyrillic (Serbian)', 'Београд'],
  ['Greek', 'Αθήνα'],
  ['Vietnamese', 'Đà Nẵng'],
  ['Latin Ext-A', 'Kraków'],
  ['Latin baseline', 'Phoenix'],
]

/** Cases `lib/og/brand.ts` documents as STILL reaching a third party. */
const UNCOVERED: Array<[string, string]> = [
  ['CJK', '東京'],
  ['Hangul', '서울'],
  ['Emoji', 'Chicago 🔥'],
]

const emitIdx = process.argv.indexOf('--emit-png')
const emitDir = emitIdx > -1 ? process.argv[emitIdx + 1] : null
if (emitDir) mkdirSync(emitDir, { recursive: true })

const realFetch = globalThis.fetch.bind(globalThis)

async function render(label: string, text: string, slug: string) {
  const hits: string[] = []
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    if (THIRD_PARTY.some(h => url.includes(h))) hits.push(url)
    return realFetch(input as RequestInfo, init)
  }) as typeof fetch

  try {
    const res = new ImageResponse(
      (
        <div
          style={{
            width: '100%',
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center',
            backgroundColor: '#0d0805',
            padding: '64px 72px',
          }}
        >
          <div style={{ display: 'flex', fontFamily: 'Space Mono', fontSize: 40, color: '#e89960' }}>
            {label}
          </div>
          <div
            style={{
              display: 'flex',
              fontFamily: 'Satoshi',
              fontWeight: 700,
              fontSize: 132,
              color: '#eee7d9',
              letterSpacing: -3,
            }}
          >
            {text}
          </div>
          <div style={{ display: 'flex', fontFamily: 'Satoshi', fontWeight: 400, fontSize: 36, color: '#9c8c7c' }}>
            {text} · 12 shows this week
          </div>
        </div>
      ),
      { width: 1200, height: 630, fonts }
    )
    const png = Buffer.from(await res.arrayBuffer())
    if (emitDir) writeFileSync(join(emitDir, `${slug}.png`), png)
    return { hits, bytes: png.length }
  } finally {
    globalThis.fetch = realFetch
  }
}

let failures = 0
console.log('\n=== COVERED — must make NO third-party request ===')
for (const [label, text] of COVERED) {
  const slug = label.toLowerCase().replace(/[^a-z0-9]+/g, '-')
  const { hits, bytes } = await render(label, text, `covered-${slug}`)
  const ok = hits.length === 0
  if (!ok) failures++
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(22)} ${JSON.stringify(text).padEnd(14)} ` +
    `png=${String(bytes).padStart(7)}B  outbound=${hits.length}`)
  hits.forEach(h => console.log(`        -> ${h.slice(0, 130)}`))
}

console.log('\n=== UNCOVERED — documented residual gap, DOES reach a third party ===')
for (const [label, text] of UNCOVERED) {
  const slug = label.toLowerCase().replace(/[^a-z0-9]+/g, '-')
  const { hits, bytes } = await render(label, text, `uncovered-${slug}`)
  const leaks = hits.length > 0
  if (!leaks) {
    failures++
    console.log(`STALE ${label.padEnd(22)} no longer fetches — update brand.ts residual gap`)
    continue
  }
  console.log(`GAP   ${label.padEnd(22)} ${JSON.stringify(text).padEnd(14)} ` +
    `png=${String(bytes).padStart(7)}B  outbound=${hits.length}`)
  hits.forEach(h => console.log(`        -> ${h.slice(0, 130)}`))
}

console.log(failures === 0 ? '\nOK — coverage and documented gap both hold.\n' : `\n${failures} FAILURE(S)\n`)
process.exit(failures === 0 ? 0 : 1)
