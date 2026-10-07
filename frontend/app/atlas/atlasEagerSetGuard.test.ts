import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

/**
 * The scripts `/atlas` loads before its map are the client graph of the root
 * layout plus the Atlas page. Turbopack puts every `'use client'` module that
 * graph can reach through a static import or re-export into those eager chunks,
 * and it does not tree-shake a barrel per export: importing one name from
 * `@/features/shows` ships everything that barrel lists. One barrel import in
 * the chrome is therefore enough to put a form library on every route.
 *
 * This test rebuilds that graph from source the way Turbopack walks it (static
 * imports and re-exports, type-only ones skipped, dynamic `import()` left out
 * because it becomes a lazy chunk, barrels followed in full) and asserts the
 * modules below are not in it. `next build` does not run in CI, so this is the
 * guard. If it fails, import the name from the file that defines it instead of
 * from the barrel; do not add the module to the allow list.
 */

const ROOT = path.resolve(__dirname, '..', '..')

const ENTRIES = [
  'app/layout.tsx',
  'app/atlas/page.tsx',
  'app/global-error.tsx',
  // Runs on every page as client code, without a directive.
  'instrumentation-client.ts',
]

/** Kept out of the `/atlas` eager set: none of them renders before the map. */
const FORBIDDEN: ReadonlyArray<readonly [label: string, matches: (id: string) => boolean]> = [
  ['zod', id => id === 'npm:zod'],
  ['TanStack Form', id => id === 'npm:@tanstack/react-form' || id === 'npm:@tanstack/form-core'],
  ['Radix Select', id => id === 'npm:@radix-ui/react-select' || id === 'components/ui/select.tsx'],
  ['ShowForm', id => id === 'features/shows/components/ShowForm.tsx'],
  [
    'the notifications feature barrels and its filter UI',
    id =>
      id === 'features/notifications/index.ts' ||
      id === 'features/notifications/components/index.ts' ||
      id.startsWith('features/notifications/components/Filter'),
  ],
  ['the auth settings barrel', id => id === 'features/auth/components/settings/index.ts'],
]

const EXTENSIONS = ['.ts', '.tsx', '.js', '.mjs', '/index.ts', '/index.tsx', '/index.js']

function resolve(from: string, specifier: string): string | null {
  let base: string
  if (specifier.startsWith('@/')) base = path.join(ROOT, specifier.slice(2))
  else if (specifier.startsWith('.')) base = path.resolve(path.dirname(from), specifier)
  else {
    const parts = specifier.split('/')
    return 'npm:' + (specifier.startsWith('@') ? parts.slice(0, 2).join('/') : parts[0])
  }
  if (fs.existsSync(base) && fs.statSync(base).isFile()) return base
  for (const extension of EXTENSIONS) if (fs.existsSync(base + extension)) return base + extension
  return null
}

interface ModuleInfo {
  useClient: boolean
  dependencies: string[]
}

const moduleCache = new Map<string, ModuleInfo>()

function readModule(file: string): ModuleInfo {
  const cached = moduleCache.get(file)
  if (cached) return cached
  const source = ts.createSourceFile(
    file,
    fs.readFileSync(file, 'utf8'),
    ts.ScriptTarget.Latest,
    false,
    file.endsWith('x') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  )
  const info: ModuleInfo = { useClient: false, dependencies: [] }
  const typeOnlyNamed = (bindings: ts.NamedImports | ts.NamedExports) =>
    bindings.elements.length > 0 && bindings.elements.every(element => element.isTypeOnly)
  for (const statement of source.statements) {
    if (
      ts.isExpressionStatement(statement) &&
      ts.isStringLiteral(statement.expression) &&
      statement.expression.text === 'use client'
    ) {
      info.useClient = true
    }
    let specifier: ts.Expression | undefined
    if (ts.isImportDeclaration(statement)) {
      const clause = statement.importClause
      if (clause?.isTypeOnly) continue
      if (clause && !clause.name && clause.namedBindings && ts.isNamedImports(clause.namedBindings) && typeOnlyNamed(clause.namedBindings)) continue
      specifier = statement.moduleSpecifier
    } else if (ts.isExportDeclaration(statement) && statement.moduleSpecifier) {
      if (statement.isTypeOnly) continue
      if (statement.exportClause && ts.isNamedExports(statement.exportClause) && typeOnlyNamed(statement.exportClause)) continue
      specifier = statement.moduleSpecifier
    }
    if (!specifier || !ts.isStringLiteral(specifier)) continue
    const target = resolve(file, specifier.text)
    if (target) info.dependencies.push(target)
  }
  moduleCache.set(file, info)
  return info
}

/** Client module ids (repo-relative paths, or `npm:<package>`), each with one import chain. */
function atlasClientGraph(): Map<string, string[]> {
  const clientChains = new Map<string, string[]>()
  const seen = new Set<string>()
  const relative = (file: string) => (file.startsWith('npm:') ? file : path.relative(ROOT, file))
  const queue: Array<[file: string, inClient: boolean, chain: string[]]> = ENTRIES.map(entry => [
    path.join(ROOT, entry),
    entry === 'instrumentation-client.ts',
    [entry],
  ])
  while (queue.length > 0) {
    const [file, inClient, chain] = queue.shift()!
    const key = `${file}#${inClient ? 'client' : 'server'}`
    if (seen.has(key)) continue
    seen.add(key)
    if (file.startsWith('npm:')) {
      if (inClient && !clientChains.has(file)) clientChains.set(file, chain)
      continue
    }
    const info = readModule(file)
    const client = inClient || info.useClient
    if (client && !clientChains.has(relative(file))) clientChains.set(relative(file), chain)
    for (const dependency of info.dependencies) {
      queue.push([dependency, client, [...chain, relative(dependency)]])
    }
  }
  return clientChains
}

describe('the /atlas eager client graph', () => {
  const graph = atlasClientGraph()

  for (const [label, matches] of FORBIDDEN) {
    it(`does not contain ${label}`, () => {
      const hits = [...graph.entries()].filter(([id]) => matches(id))
      expect(
        hits.map(([id, chain]) => `${id}\n    via ${chain.join('\n    -> ')}`),
        `${label} is reachable from the root layout or the Atlas page`,
      ).toEqual([])
    })
  }

  // The walk must actually see the chrome and the Atlas, or every absence
  // above would pass vacuously.
  it('reaches the chrome and the Atlas through the same walk', () => {
    expect(graph.has('components/layout/CommandPalette.tsx')).toBe(true)
    expect(graph.has('components/layout/nav/BottomTabBar.tsx')).toBe(true)
    expect(graph.has('features/scenes/components/AtlasGlobe.tsx')).toBe(true)
    expect(graph.has('features/notifications/components/NotificationBell.tsx')).toBe(true)
  })

  // A barrel import from a client module is the failure this guards against,
  // so the walk must follow re-exports in full.
  it('follows a barrel through all of its re-exports', () => {
    const barrel = readModule(path.join(ROOT, 'features/shows/index.ts'))
    expect(barrel.dependencies.length).toBeGreaterThan(1)
  })
})
