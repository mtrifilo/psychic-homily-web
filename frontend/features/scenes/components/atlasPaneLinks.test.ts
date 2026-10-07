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
 * - nothing under features/scenes imports from components/layout, and the
 *   link hold the pane shares with the chrome (lib/atlasMapReadyLink.tsx, with
 *   the signal it reads) imports from neither components nor features.
 */

const FRONTEND = path.resolve(__dirname, '..', '..', '..')
const COMPONENTS = __dirname
const PANE_ROOT = 'AtlasGlobe.tsx'
const PANE_LINK = 'AtlasPaneLink.tsx'

function parse(file: string, text = fs.readFileSync(file, 'utf8')): ts.SourceFile {
  return ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true)
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
 * Source files under `roots` whose text contains `needle`, with that text: one
 * recursive directory listing per root, one read of each candidate, and no
 * parse, so only files that can import what `needle` names reach the AST.
 */
function sourceFilesMentioning(roots: string[], needle: string): Array<{ file: string; text: string }> {
  return roots.flatMap((root) => {
    const base = path.join(FRONTEND, root)
    return (fs.readdirSync(base, { recursive: true }) as string[])
      .filter((rel) => /\.tsx?$/.test(rel) && !NON_SOURCE.test(rel) && !SKIPPED_SEGMENT.test(rel))
      .map((rel) => {
        const file = path.join(base, rel)
        return { file, text: fs.readFileSync(file, 'utf8') }
      })
      .filter(({ text }) => text.includes(needle))
  })
}

/** Whether `specifier`, imported from `file`, names a module under `dir`. */
function importsFrom(file: string, specifier: string, dir: string): boolean {
  const target = specifier.startsWith('@/')
    ? path.join(FRONTEND, specifier.slice(2))
    : specifier.startsWith('.')
      ? path.resolve(path.dirname(file), specifier)
      : null
  const base = path.join(FRONTEND, dir)
  return target !== null && (target === base || target.startsWith(base + path.sep))
}

/** `file: specifier` for each import in `text` (type-only included) that names a module under one of `dirs`. */
function importsUnder(file: string, text: string, dirs: string[]): string[] {
  return ts
    .preProcessFile(text, true, true)
    .importedFiles.filter(({ fileName }) => dirs.some((dir) => importsFrom(file, fileName, dir)))
    .map(({ fileName }) => `${path.relative(FRONTEND, file)}: ${fileName}`)
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
      .filter(({ file, text }) =>
        runtimeImports(parse(file, text)).some((specifier) => /(^|\/)GlobeCanvas$/.test(specifier)) &&
        /\bimport\(\s*['"][^'"]*GlobeCanvas['"]\s*\)/.test(text),
      )
      .map(({ file }) => path.relative(FRONTEND, file))
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
  // A test rather than a lint rule: a no-restricted-imports block over
  // features/scenes would replace the pane block's next/link options for the
  // files both blocks match (see the note in eslint.config.mjs).
  it('imports nothing from components/layout', { timeout: 30_000 }, () => {
    // Type-only imports count too: the rule is the dependency direction.
    const offenders = sourceFilesMentioning(['features/scenes'], 'components/layout').flatMap(({ file, text }) =>
      importsUnder(file, text, ['components/layout']),
    )
    expect(offenders).toEqual([])
  })

  it('keeps the shared link hold free of components and features', () => {
    const offenders = ['lib/atlasMapReadyLink.tsx', 'lib/atlasMapReady.ts'].flatMap((rel) => {
      const file = path.join(FRONTEND, rel)
      return importsUnder(file, fs.readFileSync(file, 'utf8'), ['components', 'features'])
    })
    expect(offenders).toEqual([])
  })
})
