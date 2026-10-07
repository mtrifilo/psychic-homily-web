import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

/**
 * Source guards for two Atlas rules that no runtime test can see:
 * - every component the Atlas pane renders from this folder links through
 *   AtlasPaneLink, never `next/link` directly, and is listed in the
 *   eslint.config.mjs block that says so;
 * - loadGlobeCanvas.ts is the only dynamic import of GlobeCanvas, so
 *   next/dynamic and the preload share one chunk group.
 */

const FRONTEND = path.resolve(__dirname, '..', '..', '..')
const COMPONENTS = __dirname
const PANE_ROOT = 'AtlasGlobe.tsx'
const PANE_LINK = 'AtlasPaneLink.tsx'

function parse(file: string): ts.SourceFile {
  return ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true)
}

/** Module specifiers a file imports at runtime: value imports and `import()`. */
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

/** A `./X` specifier resolved to a file in this folder, or null. */
function resolveSibling(specifier: string): string | null {
  if (!specifier.startsWith('./')) return null
  for (const ext of ['.tsx', '.ts']) {
    const file = path.join(COMPONENTS, `${specifier.slice(2)}${ext}`)
    if (fs.existsSync(file)) return file
  }
  return null
}

/** The `.tsx` files in this folder reachable from AtlasGlobe through `./` imports. */
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

function sourceFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) return entry.name === 'node_modules' ? [] : sourceFiles(full)
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [full] : []
  })
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
  it('is loadGlobeCanvas.ts, outside tests', () => {
    const importers = ['app', 'components', 'features', 'lib']
      .flatMap((dir) => sourceFiles(path.join(FRONTEND, dir)))
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
