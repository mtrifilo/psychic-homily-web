import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * The Account sheet's legal rows (Cookie preferences, Privacy, Terms) are the
 * phone entry to them on routes that hide the site footer below `xl`, so every
 * route must render the BottomTabBar that hosts the sheet. That holds while:
 * - app/layout.tsx is the only layout that renders the document (`<html>`),
 *   so every route renders inside it;
 * - it renders `{children}` inside AppShell;
 * - AppShell renders the BottomTabBar (AppShell.test.tsx pins both nav modes);
 * - the sheet renders the rows for every viewer on any pathname
 *   (BottomTabBar.test.tsx).
 * This file checks the first two over the source tree. `global-error.tsx` is
 * the one other document Next can render; it replaces the root layout on an
 * unrecoverable error and renders no chrome at all.
 */

const APP = path.resolve(__dirname, '..', '..', '..', 'app')

function layoutFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) return layoutFiles(full)
    return /^(layout|template)\.tsx?$/.test(entry.name) ? [full] : []
  })
}

describe('phone reach of the Account sheet legal rows', () => {
  it('renders the document only from the root layout', () => {
    const documentLayouts = layoutFiles(APP)
      .filter(file => /<html[\s>]/.test(fs.readFileSync(file, 'utf8')))
      .map(file => path.relative(APP, file))
    expect(documentLayouts).toEqual(['layout.tsx'])
  })

  it('renders every route inside AppShell', () => {
    const root = fs.readFileSync(path.join(APP, 'layout.tsx'), 'utf8')
    const shell = root.match(/<AppShell>([\s\S]*?)<\/AppShell>/)
    expect(shell, 'app/layout.tsx must wrap the routes in <AppShell>').not.toBeNull()
    expect(shell![1]).toContain('{children}')
  })
})
