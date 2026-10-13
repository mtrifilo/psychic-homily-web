import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const BACKGROUND_DECLARATION = /^\s*--background:\s*(#[0-9a-f]{6});/m

/**
 * Test-only reader for the page background colour of each scheme in
 * app/globals.css: the first `--background` before the `.dark {` block (the
 * light :root palette) and the first one inside it. Metadata that names a page
 * colour is asserted against these, so a palette change fails the metadata
 * tests instead of leaving the browser tint and the manifest behind.
 */
export function pageBackgroundTokens(): { light: string; dark: string } {
  const css = readFileSync(join(__dirname, '..', '..', 'app', 'globals.css'), 'utf8')
  const darkStart = css.indexOf('.dark {')
  const light = BACKGROUND_DECLARATION.exec(css.slice(0, darkStart))?.[1]
  const dark = BACKGROUND_DECLARATION.exec(css.slice(darkStart))?.[1]
  if (darkStart < 0 || !light || !dark) {
    throw new Error('app/globals.css: expected a --background before and inside .dark {')
  }
  return { light, dark }
}
