import { connection } from 'next/server'

/**
 * The IndexNow key file. The public address is `/{key}.txt` at the site root,
 * which next.config.ts rewrites here; the root is the only directory whose key
 * file vouches for every page on the host.
 *
 * It answers the key as text/plain when the requested key equals
 * `INDEXNOW_KEY`, and 404 for every other key and whenever the variable is
 * unset. The key is public by design (search engines fetch it to confirm the
 * host submitted the URLs), so a plain comparison is enough; the value lives in
 * the environment so each deployment holds its own.
 *
 * `await connection()` makes the handler read the variable per request rather
 * than once at build time.
 */
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ key: string }> }
): Promise<Response> {
  await connection()
  const { key } = await params
  const expected = process.env.INDEXNOW_KEY?.trim()
  if (!expected || key !== expected) {
    return new Response('Not Found', {
      status: 404,
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
    })
  }
  return new Response(expected, {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  })
}
