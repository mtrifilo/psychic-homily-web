import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const backendRoot = resolve(import.meta.dirname, '../../backend')

/**
 * Read a JSON corpus that a backend Go test reads too, at a path relative to
 * the repo's `backend/` directory
 * (`internal/utils/testdata/social_link_corpus.json`), so a suite names the
 * same path whatever folder it sits in. Go's `testcorpus.Load` resolves from
 * the same directory, so both readers of one corpus pass the same string.
 *
 * Read at runtime, never imported: an `import` of a backend JSON file would
 * pull it into the frontend TypeScript program, and `next build` typechecks
 * that program, so a Vercel project rooted at frontend/ without "include
 * source files outside the root directory" would fail the build on a test
 * fixture. A runtime read keeps the one shared file invisible to tsc and to
 * the bundle.
 *
 * `T` is the caller's statement of the corpus shape. Nothing checks the parsed
 * value against it, so each suite asserts the fields it relies on, a non-empty
 * case list at the least: an empty array turns `it.each` into zero tests.
 *
 * Throws, naming the resolved path, when the file cannot be read or parsed.
 */
export function loadBackendCorpus<T>(pathUnderBackend: string): T {
  const path = resolve(backendRoot, pathUnderBackend)
  let text: string
  try {
    text = readFileSync(path, 'utf8')
  } catch (cause) {
    throw new Error(`Cannot read backend corpus ${path}`, { cause })
  }
  try {
    return JSON.parse(text) as T
  } catch (cause) {
    throw new Error(`Cannot parse backend corpus ${path}`, { cause })
  }
}
