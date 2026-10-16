import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the eslint.config.mjs block that bans bare invalidates
 * outside refetchAfterMutation. ESLint does not report a `files` entry that
 * matches nothing, so a renamed file or a mistyped glob would silently leave
 * the rule; and a file outside the hooks trees that refetches after a write is
 * only protected once it is listed.
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

const { files, ignores } = lintBlock()

const covered = (file: string) =>
  files.some(pattern => path.matchesGlob(file, pattern)) &&
  !ignores.some(pattern => path.matchesGlob(file, pattern))

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
    for (const pattern of files) {
      expect(
        sourceFiles.some(file => path.matchesGlob(file, pattern)),
        pattern
      ).toBe(true)
    }
  })

  it('covers every file that refetches through the helper or the factory', () => {
    const refetches =
      /from\s+['"]@\/lib\/refetchAfterMutation['"]|createInvalidateQueries\(/
    const adopters = sourceFiles.filter(
      file =>
        file !== path.join('lib', 'refetchAfterMutation.ts') &&
        refetches.test(fs.readFileSync(path.join(FRONTEND, file), 'utf8'))
    )
    expect(adopters.length).toBeGreaterThan(0)
    expect(adopters.filter(file => !covered(file))).toEqual([])
  })
})
