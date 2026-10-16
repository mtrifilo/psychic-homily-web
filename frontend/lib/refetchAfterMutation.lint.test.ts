import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the eslint.config.mjs block that bans bare invalidates
 * outside refetchAfterMutation. ESLint does not report a `files` entry that
 * matches nothing, so a renamed file or a mistyped glob would silently leave
 * the rule; and a file outside the hooks trees is only linted once it is
 * listed, so any source file that refetches (through the helper, the factory
 * or a bare call) must be covered by the block.
 */

const FRONTEND = path.resolve(__dirname, '..')

function lintBlock(): { files: string[]; ignores: string[] } {
  const config = fs.readFileSync(path.join(FRONTEND, 'eslint.config.mjs'), 'utf8')
  const start = config.indexOf('Mutation hooks refetch through refetchAfterMutation')
  if (start < 0) throw new Error('eslint.config.mjs has no refetchAfterMutation block')
  const block = config.slice(start)
  const list = (name: string) => {
    const match = block.match(new RegExp(`${name}:\\s*\\[([^\\]]*)\\]`))
    if (!match) throw new Error(`the refetchAfterMutation block has no ${name} entry`)
    return [...match[1].matchAll(/"([^"]+)"/g)].map(m => m[1])
  }
  return { files: list('files'), ignores: list('ignores') }
}

/**
 * The subset of ESLint's glob syntax the block uses: `**` spans directories
 * and `*` stays within one. Anything else throws, so a pattern this cannot
 * read fails the test instead of matching nothing.
 */
function globMatcher(glob: string): (file: string) => boolean {
  if (/[?[\]{}!]/.test(glob)) throw new Error(`unsupported glob syntax: ${glob}`)
  const escape = (s: string) => s.replace(/[.+^$()|\\]/g, '\\$&')
  const source = glob
    .split('**/')
    .map(part =>
      part
        .split('**')
        .map(segment => segment.split('*').map(escape).join('[^/]*'))
        .join('.*')
    )
    .join('(?:.*/)?')
  const pattern = new RegExp(`^${source}$`)
  return file => pattern.test(file.split(path.sep).join('/'))
}

const { files, ignores } = lintBlock()
const fileMatchers = files.map(globMatcher)
const ignoreMatchers = ignores.map(globMatcher)

const covered = (file: string) =>
  fileMatchers.some(matches => matches(file)) &&
  !ignoreMatchers.some(matches => matches(file))

const sourceFiles = ['app', 'components', 'features', 'lib']
  .flatMap(dir =>
    fs
      .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
      .map(f => path.join(dir, f))
  )
  .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))

describe('the refetchAfterMutation lint block', () => {
  it('names only files and globs that match a source file', () => {
    expect(files.length).toBeGreaterThan(0)
    files.forEach((pattern, i) => {
      expect(sourceFiles.some(fileMatchers[i]), pattern).toBe(true)
    })
  })

  it('covers every file that refetches: through the helper, the factory, or a bare call', () => {
    const refetches =
      /from\s+['"][^'"]*refetchAfterMutation['"]|createInvalidateQueries\(|\b(invalidate|refetch)Queries\s*\(/
    // Comments may name the methods; only code counts.
    const code = (source: string) =>
      source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
    const adopters = sourceFiles.filter(
      file =>
        file !== path.join('lib', 'refetchAfterMutation.ts') &&
        refetches.test(code(fs.readFileSync(path.join(FRONTEND, file), 'utf8')))
    )
    expect(adopters.length).toBeGreaterThan(0)
    expect(adopters.filter(file => !covered(file))).toEqual([])
  })
})
