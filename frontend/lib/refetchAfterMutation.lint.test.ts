import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the eslint.config.mjs block that bans bare invalidates in
 * the files that refetch through refetchAfterMutation. ESLint does not report
 * a `files` entry that matches nothing, so a renamed hook file would silently
 * leave the rule; and a file that adopts the helper is only protected once it
 * is listed.
 */

const FRONTEND = path.resolve(__dirname, '..')

function lintedFiles(): string[] {
  const config = fs.readFileSync(path.join(FRONTEND, 'eslint.config.mjs'), 'utf8')
  const block = config.indexOf('refetch through refetchAfterMutation')
  if (block < 0) throw new Error('eslint.config.mjs has no refetchAfterMutation block')
  const files = config.slice(block).match(/files:\s*\[([^\]]*)\]/)
  if (!files) throw new Error('the refetchAfterMutation block has no files entry')
  return [...files[1].matchAll(/"([^"]+)"/g)].map(m => m[1])
}

function sourceFilesUnder(dir: string): string[] {
  return fs
    .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
    .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))
    .map(f => path.join(dir, f))
}

describe('the refetchAfterMutation lint block', () => {
  it('names only files that exist', () => {
    const files = lintedFiles()
    expect(files.length).toBeGreaterThan(0)
    for (const file of files) {
      expect(fs.existsSync(path.join(FRONTEND, file)), file).toBe(true)
    }
  })

  it('names every feature file that refetches through the helper', () => {
    const listed = new Set(lintedFiles())
    const adopters = sourceFilesUnder('features').filter(file =>
      fs
        .readFileSync(path.join(FRONTEND, file), 'utf8')
        .includes("from '@/lib/refetchAfterMutation'")
    )
    expect(adopters.length).toBeGreaterThan(0)
    expect(adopters.filter(file => !listed.has(file))).toEqual([])
  })
})
