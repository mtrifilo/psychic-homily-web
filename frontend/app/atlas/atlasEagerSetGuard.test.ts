import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

/**
 * Keeps named modules out of the scripts every route loads before it is
 * interactive (the root layout's client graph) and out of what `/atlas` loads
 * before its map (that graph plus the Atlas page's).
 *
 * Turbopack ships every `'use client'` module a route's server tree can reach
 * through static imports and re-exports, and it does not tree-shake a
 * `'use client'` barrel per export (features/sharedChunkBarrelGuard.test.ts).
 * This walk is the conservative version of that rule: it follows every static
 * import and re-export, barrels included in full, and every `next/dynamic`
 * import that renders on the server (Next preloads its chunk); it skips
 * type-only imports and `import()` that stays lazy (`ssr: false`, or outside
 * `dynamic()`). Where Turbopack is smarter, the walk over-reports. npm
 * packages are leaves: a package's own dependencies are not walked.
 *
 * `next build` does not run in CI, so this file is the guard. The atlas-perf
 * workflow measures the real bytes on a preview, but its path filter does not
 * cover the chrome.
 *
 * When a check fails, the message prints the import chain. Cut it at the first
 * barrel (`index.ts`) after a chrome or Atlas module: import the name from the
 * file that defines it. If the module itself is now needed before first paint,
 * load it with `dynamic()` or `import()` behind the interaction that needs it,
 * or re-measure with `bun run perf:atlas` and change the list on purpose.
 * features/sharedChunkBarrelGuard.test.ts keeps the barrels themselves lean in
 * case one becomes reachable again.
 */

const ROOT = path.resolve(__dirname, '..', '..')
const relative = (file: string) => (file.startsWith('npm:') ? file : path.relative(ROOT, file))

/** Route segment files Next renders for a route, if they exist. */
const SEGMENT_FILES = ['layout', 'template', 'page', 'loading', 'error', 'not-found'].flatMap(name => [
  `${name}.tsx`,
  `${name}.ts`,
])

function segmentFiles(dir: string): string[] {
  return SEGMENT_FILES.map(file => path.join(dir, file))
    .filter(file => fs.existsSync(file))
    .map(relative)
}

/** What every route loads: the root segment, the global error page and client instrumentation. */
const ROOT_ENTRIES = [
  ...segmentFiles(path.join(ROOT, 'app')).filter(file => !file.startsWith('app/page.')),
  'app/global-error.tsx',
  'instrumentation-client.ts',
]

/** What `/atlas` adds. */
const ATLAS_ENTRIES = [...ROOT_ENTRIES, ...segmentFiles(path.join(ROOT, 'app/atlas'))]

/** Runs as client code on every page, without a directive. */
const CLIENT_WITHOUT_DIRECTIVE = new Set(['instrumentation-client.ts'])

type Matcher = (id: string) => boolean

/** Out of the `/atlas` client graph: none of these renders before the map. */
const FORBIDDEN_ON_ATLAS: ReadonlyArray<readonly [label: string, matches: Matcher]> = [
  ['zod', id => id === 'npm:zod'],
  ['TanStack Form', id => id === 'npm:@tanstack/react-form'],
  // The repo imports Radix primitives by name from the `radix-ui` package.
  ['Radix Select', id => id === 'npm:radix-ui/Select' || id === 'components/ui/select.tsx'],
  ['ShowForm', id => id === 'features/shows/components/ShowForm.tsx'],
  [
    'the shows and shared barrels (submit and admin shows siblings ride them)',
    id =>
      id === 'features/shows/index.ts' ||
      id === 'features/shows/components/index.ts' ||
      id === 'features/shows/hooks/index.ts' ||
      id === 'components/shared/index.ts',
  ],
  ['admin hooks', id => id.startsWith('lib/hooks/admin/')],
  [
    'notifications modules beyond the bell and its unread count',
    id =>
      id.startsWith('features/notifications/') &&
      ![
        'features/notifications/components/NotificationBell.tsx',
        'features/notifications/components/NotificationList.tsx',
        'features/notifications/components/NoNotificationsYet.tsx',
        'features/notifications/hooks/index.ts',
        'features/notifications/types.ts',
      ].includes(id),
  ],
  ['the auth settings barrel', id => id === 'features/auth/components/settings/index.ts'],
]

/** Out of the graph every route loads. */
const FORBIDDEN_ON_EVERY_ROUTE: ReadonlyArray<readonly [label: string, matches: Matcher]> = [
  ['Atlas components (AtlasGlobe and the panels)', id => id.startsWith('features/scenes/components/')],
  ['shows components', id => id.startsWith('features/shows/components/')],
]

const SOURCE_EXTENSIONS = ['.ts', '.tsx', '.js', '.mjs', '/index.ts', '/index.tsx', '/index.js']
const NON_SOURCE = /\.(css|scss|svg|png|jpe?g|webp|gif|json|woff2?)$/

interface Edge {
  target: string
  /** Named imports of an npm package, for packages that ship primitives by name. */
  names: string[]
}

interface ModuleInfo {
  useClient: boolean
  edges: Edge[]
  unresolved: string[]
}

function resolve(from: string, specifier: string): string | null {
  let base: string
  if (specifier.startsWith('@/')) base = path.join(ROOT, specifier.slice(2))
  else if (specifier.startsWith('.')) base = path.resolve(path.dirname(from), specifier)
  else {
    const parts = specifier.split('/')
    return 'npm:' + (specifier.startsWith('@') ? parts.slice(0, 2).join('/') : parts[0])
  }
  if (fs.existsSync(base) && fs.statSync(base).isFile()) return base
  for (const extension of SOURCE_EXTENSIONS) if (fs.existsSync(base + extension)) return base + extension
  return null
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
  const info: ModuleInfo = { useClient: false, edges: [], unresolved: [] }
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
    let names: string[] = []
    if (ts.isImportDeclaration(statement)) {
      const clause = statement.importClause
      if (clause?.isTypeOnly) continue
      if (clause?.namedBindings && ts.isNamedImports(clause.namedBindings)) {
        if (!clause.name && typeOnlyNamed(clause.namedBindings)) continue
        names = clause.namedBindings.elements
          .filter(element => !element.isTypeOnly)
          .map(element => (element.propertyName ?? element.name).text)
      }
      specifier = statement.moduleSpecifier
    } else if (ts.isExportDeclaration(statement) && statement.moduleSpecifier) {
      if (statement.isTypeOnly) continue
      if (statement.exportClause && ts.isNamedExports(statement.exportClause) && typeOnlyNamed(statement.exportClause)) continue
      specifier = statement.moduleSpecifier
    }
    if (!specifier || !ts.isStringLiteral(specifier) || NON_SOURCE.test(specifier.text)) continue
    addEdge(info, file, specifier.text, names)
  }
  for (const specifier of preloadedDynamicImports(source)) addEdge(info, file, specifier, [])
  moduleCache.set(file, info)
  return info
}

function addEdge(info: ModuleInfo, file: string, specifier: string, names: string[]) {
  const target = resolve(file, specifier)
  if (target) info.edges.push({ target, names })
  else info.unresolved.push(specifier)
}

/**
 * `next/dynamic` renders on the server by default, and on the server Next
 * preloads the chunk (a `<link rel=preload>` in the HTML), so the module loads
 * with the page. Only `{ ssr: false }` keeps it lazy. Returns the `import()`
 * targets of `dynamic()` calls that do not pass a literal `ssr: false`.
 */
function preloadedDynamicImports(source: ts.SourceFile): string[] {
  const dynamicNames = new Set<string>()
  for (const statement of source.statements) {
    if (
      ts.isImportDeclaration(statement) &&
      ts.isStringLiteral(statement.moduleSpecifier) &&
      statement.moduleSpecifier.text === 'next/dynamic' &&
      statement.importClause?.name
    ) {
      dynamicNames.add(statement.importClause.name.text)
    }
  }
  if (dynamicNames.size === 0) return []
  const targets: string[] = []
  const importsIn = (node: ts.Node, out: string[]) => {
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      out.push(node.arguments[0].text)
    }
    ts.forEachChild(node, child => importsIn(child, out))
  }
  const visit = (node: ts.Node) => {
    if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && dynamicNames.has(node.expression.text)) {
      const options = node.arguments[1]
      const ssrOff =
        options !== undefined &&
        ts.isObjectLiteralExpression(options) &&
        options.properties.some(
          property =>
            ts.isPropertyAssignment(property) &&
            ts.isIdentifier(property.name) &&
            property.name.text === 'ssr' &&
            property.initializer.kind === ts.SyntaxKind.FalseKeyword,
        )
      if (!ssrOff && node.arguments[0]) importsIn(node.arguments[0], targets)
    }
    ts.forEachChild(node, visit)
  }
  visit(source)
  return targets
}

interface Graph {
  /** Client module ids (repo-relative paths, `npm:<package>`, `npm:radix-ui/<Primitive>`), each with one import chain. */
  client: Map<string, string[]>
  /** First-party specifiers the walk could not resolve, as `file: specifier`. */
  unresolved: string[]
}

function clientGraph(entries: readonly string[]): Graph {
  const client = new Map<string, string[]>()
  const unresolved: string[] = []
  const seen = new Set<string>()
  const queue: Array<[file: string, inClient: boolean, chain: string[]]> = entries.map(entry => [
    path.join(ROOT, entry),
    CLIENT_WITHOUT_DIRECTIVE.has(entry),
    [entry],
  ])
  const record = (id: string, chain: string[]) => {
    if (!client.has(id)) client.set(id, chain)
  }
  while (queue.length > 0) {
    const [file, inClient, chain] = queue.shift()!
    const key = `${file}#${inClient ? 'client' : 'server'}`
    if (seen.has(key)) continue
    seen.add(key)
    if (file.startsWith('npm:')) {
      if (inClient) record(file, chain)
      continue
    }
    const info = readModule(file)
    for (const specifier of info.unresolved) unresolved.push(`${relative(file)}: ${specifier}`)
    const isClient = inClient || info.useClient
    if (isClient) record(relative(file), chain)
    for (const edge of info.edges) {
      const next = [...chain, relative(edge.target)]
      if (isClient && edge.target === 'npm:radix-ui') {
        for (const name of edge.names) record(`npm:radix-ui/${name}`, next)
      }
      queue.push([edge.target, isClient, next])
    }
  }
  return { client, unresolved }
}

function offenders(graph: Graph, matches: Matcher): string[] {
  return [...graph.client.entries()]
    .filter(([id]) => matches(id))
    .map(([id, chain]) => `${id}\n    via ${chain.join('\n    -> ')}`)
}

const HOW_TO_FIX =
  'cut the chain at its first barrel (index.ts) by importing from the defining file, or lazy-load the module (see the header of app/atlas/atlasEagerSetGuard.test.ts)'

describe('the /atlas eager client graph', () => {
  const graph = clientGraph(ATLAS_ENTRIES)

  for (const [label, matches] of FORBIDDEN_ON_ATLAS) {
    it(`does not contain ${label}`, () => {
      expect(offenders(graph, matches), `${label} is reachable on /atlas: ${HOW_TO_FIX}`).toEqual([])
    })
  }
})

describe('the client graph every route loads', () => {
  const graph = clientGraph(ROOT_ENTRIES)

  for (const [label, matches] of FORBIDDEN_ON_EVERY_ROUTE) {
    it(`does not contain ${label}`, () => {
      expect(offenders(graph, matches), `${label} is reachable from the root layout: ${HOW_TO_FIX}`).toEqual([])
    })
  }
})

describe('the walk itself', () => {
  it('resolves every first-party import it meets', () => {
    expect(clientGraph(ATLAS_ENTRIES).unresolved, 'an unresolved import would drop an edge silently').toEqual([])
  })

  it('reaches the chrome and the Atlas, so the absences above are not vacuous', () => {
    const graph = clientGraph(ATLAS_ENTRIES)
    for (const id of [
      'components/layout/nav/BottomTabBar.tsx',
      'components/layout/TopBar.tsx',
      'features/scenes/components/AtlasGlobe.tsx',
    ]) {
      expect(graph.client.has(id), `${id} should be in the /atlas client graph`).toBe(true)
    }
  })

  it('follows a barrel through its re-exports to the modules it lists', () => {
    const graph = clientGraph(['components/shared/index.ts'])
    // A server barrel's re-exports are client modules in their own right.
    expect(graph.client.has('components/shared/SaveButton.tsx')).toBe(true)
    expect(graph.client.has('features/shows/components/ShowForm.tsx')).toBe(true)
  })

  it('follows next/dynamic imports that render on the server, not ssr: false ones', () => {
    const fixture = ts.createSourceFile(
      'fixture.tsx',
      [
        "import dynamic from 'next/dynamic'",
        "const Eager = dynamic(() => import('./eager'))",
        "const Lazy = dynamic(() => import('./lazy'), { ssr: false })",
        "const later = () => import('./later')",
      ].join('\n'),
      ts.ScriptTarget.Latest,
      false,
      ts.ScriptKind.TSX,
    )
    expect(preloadedDynamicImports(fixture)).toEqual(['./eager'])
  })

  it('records the Radix primitives a client module imports by name', () => {
    const graph = clientGraph(['components/ui/select.tsx'])
    expect(graph.client.has('npm:radix-ui/Select')).toBe(true)
  })
})
