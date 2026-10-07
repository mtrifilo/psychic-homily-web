import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

/**
 * Source guards for three Atlas rules that no runtime test can see:
 * - every component the Atlas pane renders from this folder links through
 *   AtlasPaneLink, never `next/link` directly, and is listed in the
 *   eslint.config.mjs block that says so;
 * - loadGlobeCanvas.ts is the only dynamic import of GlobeCanvas, so
 *   next/dynamic and the preload share one chunk group;
 * - nothing under features/scenes imports from components/layout: the link
 *   hold the pane shares with the chrome lives in lib/atlasMapReadyLink.tsx.
 */

const FRONTEND = path.resolve(__dirname, '..', '..', '..')
const COMPONENTS = __dirname
const PANE_ROOT = 'AtlasGlobe.tsx'
const PANE_LINK = 'AtlasPaneLink.tsx'

function parse(file: string): ts.SourceFile {
  return ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true)
}

/**
 * Module specifiers a file loads at runtime: value imports, value re-exports
 * and `import()`.
 */
function runtimeImports(source: ts.SourceFile): string[] {
  const found: string[] = []
  const visit = (node: ts.Node) => {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      !node.importClause?.isTypeOnly
    ) {
      found.push(node.moduleSpecifier.text)
    } else if (
      ts.isExportDeclaration(node) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      !node.isTypeOnly
    ) {
      found.push(node.moduleSpecifier.text)
    } else if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      found.push(node.arguments[0].text)
    }
    ts.forEachChild(node, visit)
  }
  visit(source)
  return found
}

const SIBLING_PREFIXES = ['./', '@/features/scenes/components/']

/** A specifier for a file in this folder (`./X` or its `@/` path), resolved, or null. */
function resolveSibling(specifier: string): string | null {
  const prefix = SIBLING_PREFIXES.find((p) => specifier.startsWith(p))
  if (!prefix) return null
  const name = specifier.slice(prefix.length)
  if (name.includes('/')) return null
  for (const ext of ['.tsx', '.ts']) {
    const file = path.join(COMPONENTS, `${name}${ext}`)
    if (fs.existsSync(file)) return file
  }
  return null
}

/** The `.tsx` files in this folder that AtlasGlobe reaches through imports of them. */
function paneComponents(): string[] {
  const seen = new Set<string>()
  const queue = [path.join(COMPONENTS, PANE_ROOT)]
  while (queue.length > 0) {
    const file = queue.pop()!
    if (seen.has(file)) continue
    seen.add(file)
    for (const specifier of runtimeImports(parse(file))) {
      const next = resolveSibling(specifier)
      if (next) queue.push(next)
    }
  }
  return [...seen].filter((file) => file.endsWith('.tsx')).map((file) => path.basename(file))
}

/** The file names in the eslint.config.mjs block that points at AtlasPaneLink. */
function lintedPaneFiles(): string[] {
  const config = fs.readFileSync(path.join(FRONTEND, 'eslint.config.mjs'), 'utf8')
  const match = config.match(/"features\/scenes\/components\/\{([^}]*)\}\.tsx"/)
  if (!match) throw new Error('eslint.config.mjs has no features/scenes/components/{...}.tsx files entry')
  return match[1].split(',').map((name) => `${name.trim()}.tsx`)
}

const SCANNED_ROOTS = ['app', 'components', 'features', 'lib']
const SKIPPED_SEGMENT = /(^|[\\/])(node_modules|\.next)([\\/]|$)/
const NON_SOURCE = /\.(test|spec|stories)\.tsx?$/

/**
 * Source files under `roots` whose text contains `needle`: one recursive
 * directory listing per root, a read of each candidate, and no parse, so only
 * files that can import what `needle` names reach the AST.
 */
function sourceFilesMentioning(roots: string[], needle: string): string[] {
  return roots.flatMap((root) => {
    const base = path.join(FRONTEND, root)
    return (fs.readdirSync(base, { recursive: true }) as string[])
      .filter((rel) => /\.tsx?$/.test(rel) && !NON_SOURCE.test(rel) && !SKIPPED_SEGMENT.test(rel))
      .map((rel) => path.join(base, rel))
      .filter((file) => fs.readFileSync(file, 'utf8').includes(needle))
  })
}

const LAYOUT = path.join(FRONTEND, 'components', 'layout')

/** Whether `specifier`, imported from `file`, names a module under components/layout. */
function isLayoutModule(file: string, specifier: string): boolean {
  const target = specifier.startsWith('@/')
    ? path.join(FRONTEND, specifier.slice(2))
    : specifier.startsWith('.')
      ? path.resolve(path.dirname(file), specifier)
      : null
  return target !== null && (target === LAYOUT || target.startsWith(LAYOUT + path.sep))
}

describe('the Atlas pane links through AtlasPaneLink', () => {
  const components = paneComponents()

  it('finds the pane from AtlasGlobe', () => {
    // The walk itself: if these drop out, it has stopped following imports.
    expect(components).toEqual(
      expect.arrayContaining(['AtlasGlobe.tsx', 'ScenePreviewContent.tsx', 'VenuePanel.tsx', 'ArtistPanel.tsx', 'MyScenesStrip.tsx', 'GlobeCanvas.tsx', PANE_LINK]),
    )
  })

  it('imports next/link in no pane component', () => {
    const direct = components
      .filter((name) => name !== PANE_LINK)
      .filter((name) => runtimeImports(parse(path.join(COMPONENTS, name))).includes('next/link'))
    expect(direct).toEqual([])
  })

  it('lists every pane component in the lint block', () => {
    const linted = new Set(lintedPaneFiles())
    const missing = components.filter((name) => name !== PANE_LINK && !linted.has(name))
    expect(missing).toEqual([])
  })
})

describe('GlobeCanvas has one dynamic import', () => {
  // A scan of the frontend's source tree, which takes longer than a unit test
  // on a shared CI runner; the timeout is for that, not for anything async.
  it('is loadGlobeCanvas.ts, outside tests', { timeout: 30_000 }, () => {
    const importers = sourceFilesMentioning(SCANNED_ROOTS, 'GlobeCanvas')
      .filter((file) =>
        runtimeImports(parse(file)).some((specifier) => /(^|\/)GlobeCanvas$/.test(specifier)) &&
        /\bimport\(\s*['"][^'"]*GlobeCanvas['"]\s*\)/.test(fs.readFileSync(file, 'utf8')),
      )
      .map((file) => path.relative(FRONTEND, file))
    expect(importers).toEqual(['features/scenes/components/loadGlobeCanvas.ts'])
  })

  it('is what next/dynamic and the preload both call', () => {
    const atlasGlobe = fs.readFileSync(path.join(COMPONENTS, 'AtlasGlobe.tsx'), 'utf8')
    const preload = fs.readFileSync(path.join(COMPONENTS, 'atlasMapPreload.ts'), 'utf8')
    expect(atlasGlobe).toMatch(/dynamic\(loadGlobeCanvas,/)
    expect(preload).toMatch(/loadGlobeCanvas\(\)/)
  })
})

describe('features/scenes and components/layout', () => {
  // A scan of the feature's source tree, cheap for the same reason as the
  // GlobeCanvas scan; the timeout is for a shared CI runner, not for anything
  // async.
  it('imports nothing from components/layout', { timeout: 30_000 }, () => {
    // Type-only imports count too: the rule is the dependency direction.
    const offenders = sourceFilesMentioning(['features/scenes'], 'components/layout').flatMap((file) =>
      ts
        .preProcessFile(fs.readFileSync(file, 'utf8'), true, true)
        .importedFiles.filter(({ fileName }) => isLayoutModule(file, fileName))
        .map(({ fileName }) => `${path.relative(FRONTEND, file)}: ${fileName}`),
    )
    expect(offenders).toEqual([])
  })
})
