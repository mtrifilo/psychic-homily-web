---
name: psy-design-system
description: Resume or extend the PSY-646 Psychic Homily design system build in Figma. Use when the user asks to continue the design system, build / audit / debug a component (Button, Badge, Card, Input, Dialog, Sheet, etc.) in the design system Figma file, investigate Figma plugin API behavior in this specific file, or pick up where a prior agent's handoff brief left off. Loads project-specific context (file key, locked design direction, current build state) and accumulated gotchas not covered by `figma-use` or `figma-generate-library`.
---

# psy-design-system: Psychic Homily Figma DS workflow

PSY-646 design system overhaul — editorial / dense / newsprint aesthetic, retiring Geist. This skill is the project-specific layer on top of `figma-use` (Plugin API rules) and `figma-generate-library` (phased workflow). It encodes locked decisions and the gotchas that have already cost time.

## When to use

- User asks to continue / extend / resume the design system build
- New component build (Card, Input, Dialog, Sheet, popovers, sidebar, etc.)
- Audit / debug an existing component or token in the file
- Translating Figma → `app/globals.css` token spec, or drafting `docs/features/design-system.md` (PSY-646 deliverables)

## Critical pointers

- **Figma file**: `isfHz0oyFK1ALX19IRGg51` — `https://www.figma.com/design/isfHz0oyFK1ALX19IRGg51`
- **Plan key**: `team::1636949829108678549` (Professional tier — variable modes work, MCP quota generous)
- **Linear project**: "Design System Overhaul: Editorial & Dense"
- **Active ticket**: PSY-646 (spike). PSY-647..653 are implementation tickets gated on PSY-646 deliverables.
- **Related auto-memory** (must read before clone-then-rebind work):
  - `~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/pattern_figma_bound_paint_cache.md`

## Prerequisites — load BEFORE any `use_figma` call

1. `Skill(figma:figma-use)` — Plugin API syntax rules (mandatory)
2. `Skill(figma:figma-generate-library)` — phased workflow + component creation
3. Load Figma MCP tool schemas:
   ```
   ToolSearch select:mcp__plugin_figma_figma__use_figma,mcp__plugin_figma_figma__get_screenshot,mcp__plugin_figma_figma__get_metadata,mcp__plugin_figma_figma__whoami
   ```
4. Optional sanity check: `mcp__plugin_figma_figma__whoami` (confirms OAuth still alive)

Always pass `skillNames: "figma-generate-library,figma-use"` (plus `psy-design-system` if you want) to every `use_figma` call — logging only, but conventional.

## Locked design direction (DO NOT relitigate without strong reason)

### Typography
- **Display**: Clash Display (Fontshare, free, ITF License) — **MCP-invisible, see G2**
- **Body / UI**: Satoshi (Fontshare, free, ITF License) — **MCP-invisible, see G2**
- **Data / metadata / numerics**: Space Mono (Google Fonts) — MCP-visible, render correctly

### Color seeds — Light mode (newsprint / paper)
- `bg` `#f4f1ea` · `fg` `#1a1714` · `primary` `#d2541b` (burnt orange) · `border` `#ddd6c8`

### Color — Dark mode (vinyl record shop)
- Refined version of existing hand-tuned palette in `app/globals.css:99-132`

### Density / aesthetic principles
- **Borders over shadows** — one minimal `shadow/popover` effect style for overlays only
- **Sharp corners** — radius scale 2/4/6/8px (`radius/sm`..`radius/xl`)
- **Dense list/table default** — not card grids
- **Badges are square-ish tag chips** — `rounded-full` is BANNED
- **Information-dense "music publication" feel** — not "generic SaaS dashboard"

### Banned
`rounded-full` · soft drop shadows · slate palette · Geist · pill badges · card-grid-as-default · Claude Design (the original method choice — pivoted to Figma 2026-05-14; don't propose again without strong evidence change)

## Build state (refresh by reading `MEMORY.md` + running the file inventory below)

As of 2026-05-16 (end-of-day):
- **Phase 1 Foundations**: ✓ — 4 collections / **32 Color + 6 Spacing + 4 Radius + 3 Typography = 45 variables** / 1 effect style / 2 mono text styles. 6 Display/Body text styles deferred pending font upload (G2). `destructive/foreground` added 2026-05-16 (Light + Dark values mirror `primary/foreground`).
- **Phase 2 File structure + Foundations docs**: ✓ — 10 pages; Cover/Color/Type/Spacing&Radius populated.
- **Phase 3 Components shipped**: **Button** (18 variants, audited clean post-fix), **Badge** (4 variants, audited clean post-fix), **Card** (v1 outer shell + 5 sub-components: CardTitle, CardDescription, CardHeader [contains nested Title + Description instances], CardContent, CardFooter — all on the Card page), **Input** (4 state variants — Default/Focused/Error/Disabled, audited clean). PSY-649 scope first-batch is now complete.
- **Token decisions made 2026-05-16**: `border` Light bumped `#ddd6c8` → `#cabe9f` (more visible pencil hairline); `destructive/foreground` token added for semantic correctness on Destructive variants.
- **Phase 4 QA pass**: not started — runs after token-spec doc + remaining components in PSY-650/651/652/653 scope are decided.
- **Token spec deliverable** (`docs/features/design-system.md`): not yet drafted — gates PSY-647..653.
- **G1 audit (last run 2026-05-16, post-Card-family)**: 0 mismatches across 262 bound paints.

Always verify state — never trust handoff brief / this skill blindly. Run the inventory below.

## Verify state — read-only inventory (run before any new build)

```js
// use_figma — pass fileKey "isfHz0oyFK1ALX19IRGg51"
const pages = figma.root.children.map(p => ({ name: p.name, id: p.id, childCount: p.children?.length ?? 0 }));
const collections = await figma.variables.getLocalVariableCollectionsAsync();
const collInfo = await Promise.all(collections.map(async c => ({
  name: c.name, modes: c.modes.map(m => m.name), varCount: c.variableIds.length
})));
const components = [];
for (const page of figma.root.children) {
  await figma.setCurrentPageAsync(page);
  for (const child of page.children) {
    if (child.type === 'COMPONENT' || child.type === 'COMPONENT_SET') {
      components.push({ page: page.name, name: child.name, type: child.type, id: child.id,
        variantCount: child.type === 'COMPONENT_SET' ? child.children.length : 1 });
    }
  }
}
const textStyles = (await figma.getLocalTextStylesAsync()).map(s => s.name);
const effectStyles = (await figma.getLocalEffectStylesAsync()).map(s => s.name);
return { pages, collections: collInfo, components, textStyles, effectStyles };
```

`get_metadata` without a `nodeId` only lists the FIRST page — don't trust it for full inventory. Use the script above.

## Component-build workflow (mirror Button + Badge shape)

For each component:

1. **Plan**: which variants (axes + values), what tokens bind to what, what properties (TEXT / BOOLEAN / INSTANCE_SWAP). Cap variants at ~30 per the figma-generate-library rule. Ambiguity about WHAT to build → STOP and ask (per project CLAUDE.md `feedback_no_speculative_implementation`).
2. **Call 1 — page + doc frame + base Default variant**:
   - Create page named `<Component>` (skip if already exists).
   - Doc frame at (40, 40), 560px wide, padding 32, gap 12, transparent bg:
     - Title (Inter Semi Bold 32) — component name
     - Description (Inter Regular 14, 20 line-height) — what it is + variant count
     - `maps to components/ui/<component>.tsx` in `Mono/S` text style
   - Base `Variant=Default` component at (700, 40) with full token bindings (fills, strokes, padding, radius, text fills).
3. **Call 2 — clone + retone remaining variants**:
   - `clone()` the Default → rename → **use direct-paint-construction for fills/strokes (G1)** — never `setBoundVariableForPaint` on a cloned paint.
   - `figma.combineAsVariants([...variants], page)` → set name to `<Component>`.
   - Position variants in a grid (column spacing 178px matches Button + Badge). Resize the set frame.
4. **Call 3 — component properties + screenshot**:
   - Add `Label` TEXT property (or other TEXT/BOOLEAN/INSTANCE_SWAP properties).
   - Wire each variant's text child via `txt.componentPropertyReferences = { characters: labelPropKey }`.
   - `get_screenshot` of the page.
5. **Audit** — re-run G1 audit script (below) to confirm zero cached-color mismatches before checkpoint.
6. **User checkpoint** — present screenshot, await EXPLICIT approval before next component.

## Gotchas (read all before building)

### G1. Bound-paint cache mismatch (HIGH — has bitten this project twice)

`figma.variables.setBoundVariableForPaint(placeholder, 'color', variable)` does **NOT** refresh the paint's cached `color` field when the paint was already bound to the same variable (common after `clone()`-then-rebind). Figma renders the cached `color`, NOT the bound variable's resolved value.

**Failure mode**: clone Default (cached cream, bound to primary/foreground) → rebind to primary/foreground via setBoundVariableForPaint with placeholder gray → cache stays gray → node renders gray instead of cream.

**Fix — direct paint construction (always use this for clone-then-rebind):**

```js
const lightVal = variable.valuesByMode[lightModeId];
node.fills = [{
  type: 'SOLID', visible: true, opacity: 1, blendMode: 'NORMAL',
  color: { r: lightVal.r, g: lightVal.g, b: lightVal.b },
  boundVariables: { color: { type: 'VARIABLE_ALIAS', id: variable.id } }
}];
```

**Audit script** — walk every bound paint, compare cached `color` vs the variable's resolved value in the first mode of its collection, flag mismatches > 0.01 epsilon per channel:

```js
const allVars = await figma.variables.getLocalVariablesAsync();
const allColls = await figma.variables.getLocalVariableCollectionsAsync();
const collById = new Map(allColls.map(c => [c.id, c]));
const varById = new Map();
for (const v of allVars) {
  if (v.resolvedType !== 'COLOR') continue;
  const coll = collById.get(v.variableCollectionId);
  const val = v.valuesByMode[coll.modes[0].modeId];
  if (val && 'r' in val) varById.set(v.id, { name: v.name, resolved: { r: val.r, g: val.g, b: val.b } });
  else if (val?.type === 'VARIABLE_ALIAS') {
    const aliased = allVars.find(x => x.id === val.id);
    const aliasedColl = collById.get(aliased.variableCollectionId);
    const aliasedVal = aliased.valuesByMode[aliasedColl.modes[0].modeId];
    if (aliasedVal && 'r' in aliasedVal) varById.set(v.id, { name: v.name + ' (→ ' + aliased.name + ')', resolved: { r: aliasedVal.r, g: aliasedVal.g, b: aliasedVal.b } });
  }
}
const EPS = 0.01;
const mismatches = [];
for (const page of figma.root.children) {
  await figma.setCurrentPageAsync(page);
  page.findAll(n => {
    for (const kind of ['fills', 'strokes']) {
      if (!(kind in n) || !Array.isArray(n[kind])) continue;
      n[kind].forEach((p, i) => {
        if (p?.type !== 'SOLID' || !p.boundVariables?.color) return;
        const meta = varById.get(p.boundVariables.color.id);
        if (!meta) return;
        const d = ['r','g','b'].map(k => Math.abs(p.color[k] - meta.resolved[k]));
        if (Math.max(...d) > EPS) mismatches.push({
          page: page.name, nodeId: n.id, nodeName: n.name, kind, varName: meta.name,
          cached: p.color, resolved: meta.resolved
        });
      });
    }
    return false;
  });
}
return { mismatchCount: mismatches.length, mismatches };
```

**Already-fixed mismatches** (do not re-audit these unless suspicious): Button × 7 nodes (2 bg fills + 5 text labels), Badge × 1 node (Destructive text). Both fixed 2026-05-16.

**Scope clarification — NOT broken:** `variable.setValueForMode(modeId, newValue)` **does** auto-refresh cached colors on every bound paint across the file. Verified 2026-05-16: bumped `border` Light value via setValueForMode → immediate G1 audit found 0 mismatches across 256 paints. The G1 bug is narrowly about `setBoundVariableForPaint` failing to update the cache when REBINDING to the same variable. Variable VALUE changes (`setValueForMode`) work correctly. So you can change a token's value with confidence — the bug only bites clone-then-rebind sequences.

### G2. MCP can't see local OS fonts

Figma MCP runs in Figma's cloud sandbox, not your desktop Figma session. `figma.listAvailableFontsAsync()` returns ~1,723 families — **none are Clash Display or Satoshi** even when locally installed. Space Mono works because it's in Google Fonts.

Current workaround: Inter as fallback for Display + Body specimens, disclosure labels point to `globals.css` codebase tokens. Fully unblocks when user uploads Clash Display + Satoshi to the Figma team library (Admin → Fonts → Upload — one-time). User had not done this as of 2026-05-16.

### G3. `setSharedPluginData` not supported on VariableCollection

The figma-generate-library reference docs show a `setSharedPluginData('dsb', 'key', ...)` pattern for tagging entities. Works on scene nodes. Does NOT work on `VariableCollection` — throws "method not implemented." Use **name-based idempotency** for collections + variables: `(await figma.variables.getLocalVariablesAsync()).find(v => v.name === name)`.

### G4. `layoutSizingHorizontal/Vertical` after appendChild always

Setting these to ANY value (including FIXED) throws if the child isn't yet inside an auto-layout parent. Order: `parent.appendChild(child)` first, THEN set sizing modes. The figma-use skill says this only applies to FILL/HUG — in practice it bites FIXED too.

### G5. Each `use_figma` call is atomic — retries are safe

If a script errors, NO changes are written. Clean retry after fixing the script. Don't sprinkle defensive idempotency checks unless you genuinely need cross-run idempotency.

### G6. Never parallelize `use_figma` calls

Per the figma-use rule. Mutations must be strictly sequential. Independent `get_screenshot` / `get_metadata` reads can run concurrent with each other, but NOT alongside a `use_figma` call (the page-switch inside a `use_figma` script can race with screenshot rendering).

### G7. Pre-load EVERY font used on any page you touch

Per the figma-use rule, font loading is required before `appendChild`, `setBoundVariable`, `findAll` callbacks, etc. — not just text-setting. For Badge / Button work, preload at minimum: `Inter Regular`, `Inter Medium`, `Inter Semi Bold`, `Space Mono Regular`. Failing to preload causes silent or noisy failures on operations that don't appear text-related.

### G8. `get_metadata` without nodeId only returns first page

It will look like the file has only one page. Don't draw inventory conclusions from it. Use the inventory script in §"Verify state."

### G9. Tailwind v4 — never define `--spacing-{xs|sm|md|lg|xl|2xl}` in `@theme`

Tailwind v4 routes `max-w-*` / `min-w-*` / `w-*` / `h-*` / `min-h-*` utilities through `--spacing-*` when those named keys exist, taking PRIORITY over `--container-*` regardless of declaration order. Defining `--spacing-lg: 24px` makes `max-w-lg = 24px` instead of 32rem — site-wide breakage on any page using `max-w-lg`, `max-w-md`, etc. (admin dialogs, modal contents, marketing pages — many places).

**Discovered the hard way 2026-05-16:** translating Figma's spacing scale into `--spacing-{xs..2xl}` collapsed `max-w-lg` on `/scenes`, `/charts`, `/community/leaderboard`, every `<DialogContent className="max-w-lg">`, etc. Each word in the affected text block rendered on its own line because the container collapsed to 24px wide.

**Fix:** drop the semantic spacing scale entirely. Use Tailwind v4 defaults — they already match Figma's scale at standard numeric keys:

| Figma token | Pixel value | Tailwind class |
|---|---|---|
| `spacing/xs` | 4px | `p-1`, `gap-1`, `m-1` |
| `spacing/sm` | 8px | `p-2` |
| `spacing/md` | 16px | `p-4` |
| `spacing/lg` | 24px | `p-6` |
| `spacing/xl` | 32px | `p-8` |
| `spacing/2xl` | 48px | `p-12` |

When restyling components for PSY-649..653, translate Figma `spacing/md` → Tailwind `p-4`, etc. Do not re-introduce `--spacing-{named}` to globals.css.

**Radius scale is safe** — `rounded-*` utilities only consume `--radius-*` and don't fall through to other namespaces. Our `--radius-sm/md/lg/xl` definitions in globals.css are fine.

## Resume protocol (for new agents picking this up cold)

1. Load this skill (you're here).
2. Load `figma:figma-use` + `figma:figma-generate-library` skills.
3. Load Figma MCP tool schemas (see Prerequisites §3).
4. Sanity-check auth: `mcp__plugin_figma_figma__whoami`.
5. Run the §"Verify state" inventory script. Cross-reference with the build state in this skill — if they disagree, file state wins.
6. Pick up the next component. Mirror Button + Badge structure.
7. Use **direct paint construction (G1)** for every clone-then-rebind. Re-run the G1 audit after each component.
8. User checkpoint after every component — present screenshot, await explicit approval.

## Updating this skill

When new gotchas are discovered:
- Add to the Gotchas section (G9, G10, ...) — keep priority ordering.
- If the gotcha is a project-specific *pattern* worth indexing, also write a `pattern_*.md` in the user-level auto-memory dir and link it via `[[name]]` here.
- Bump the "as of YYYY-MM-DD" date in §"Build state" when state changes materially.
