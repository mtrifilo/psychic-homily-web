---
name: psy-design-system
description: Resume or extend the PSY-646 Psychic Homily design system build in Figma. Use when the user asks to continue the design system, build / audit / debug a component (Button, Badge, Card, Input, Dialog, Sheet, etc.) in the design system Figma file, investigate Figma plugin API behavior in this specific file, or pick up where a prior agent's handoff brief left off. Loads project-specific context (file key, locked design direction, current build state) and accumulated gotchas not covered by `figma-use` or `figma-generate-library`.
---

# psy-design-system: Psychic Homily Figma DS workflow

PSY-646 design system overhaul — editorial / dense / newsprint aesthetic, retiring Geist. This skill is the project-specific layer on top of `figma-use` (Plugin API rules), `figma-generate-library` (phased workflow), and `figma-reference` (read-tool decision tree + recovery patterns when MCP misbehaves). It encodes locked decisions and the gotchas that have already cost time.

## When to use

This skill covers TWO related workflows across two Figma files:

**A. Design system build (PSY-646 DS file `isfHz0oyFK1ALX19IRGg51`)**
- User asks to continue / extend / resume the design system build
- New component build (Card, Input, Dialog, Sheet, popovers, sidebar, etc.)
- Audit / debug an existing component or token in the file
- Translating Figma → `app/globals.css` token spec, or drafting `docs/features/design-system.md` (PSY-646 deliverables)

**B. Product-design mocks (Product Designs file `XakQQ0nYGqnt77PrHKO9IE`)**
- User asks to mock up a feature page / drawer / modal that consumes the DS as a library
- Iterating on a product-feature mock (rename, restructure, density changes) before filing impl ticket(s)
- New product-feature ticket where visual mocks unblock the impl ticket decomposition

Both workflows share the same file conventions, the same library setup, and the same gotchas — but the build steps differ. See "Component-build workflow" for A; "Product-design workflow" for B.

## Critical pointers

- **DS file**: `isfHz0oyFK1ALX19IRGg51` — `https://www.figma.com/design/isfHz0oyFK1ALX19IRGg51` (Psychic Homily Design System; published as a team library)
- **Product Designs file**: `XakQQ0nYGqnt77PrHKO9IE` — `https://www.figma.com/design/XakQQ0nYGqnt77PrHKO9IE` (Psychic Homily — Product Designs; single file, pages-per-feature, consumes the DS library)
- **Plan key**: `team::1636949829108678549` (Professional tier — variable modes work, MCP quota generous)
- **Linear projects**: "Design System Overhaul: Editorial & Dense" (DS work). Product-design mocks live in whatever Linear project the ticket belongs to (`Knowledge Graph: Discovery Web`, `Collections: Frictionless Curation`, etc).
- **Active DS ticket**: PSY-646 (spike). PSY-647..653 are implementation tickets gated on PSY-646 deliverables.
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

As of 2026-05-24 (end-of-day):
- **Phase 1 Foundations**: ✓ — 4 collections / **32 Color + 6 Spacing + 4 Radius + 3 Typography = 45 variables** / 1 effect style / 2 mono text styles. 6 Display/Body text styles deferred pending font upload (G2). `destructive/foreground` added 2026-05-16 (Light + Dark values mirror `primary/foreground`).
- **Phase 2 File structure + Foundations docs**: ✓ — 10 pages; Cover/Color/Type/Spacing&Radius populated.
- **Phase 3 Components shipped**: **Button** (18 variants, audited clean post-fix), **Badge** (4 variants, audited clean post-fix), **Card** (v1 outer shell + 5 sub-components: CardTitle, CardDescription, CardHeader [contains nested Title + Description instances], CardContent, CardFooter — all on the Card page), **Input** (4 state variants — Default/Focused/Error/Disabled, audited clean), **Textarea** (4 state variants, audited clean 2026-05-24), **Checkbox** (4 state variants — Default/Checked/Focused/Disabled, lucide checkmark vector, audited clean 2026-05-24), **TabTrigger** (3 state variants — Inactive/Active/Disabled, underline-style accent, audited clean 2026-05-24), **Sheet** (1 variant — Side=Right desktop chrome with built-in Header/Body/Footer placeholders, audited clean 2026-05-24). PSY-823 primitive set complete.
- **Token decisions made 2026-05-16**: `border` Light bumped `#ddd6c8` → `#cabe9f` (more visible pencil hairline); `destructive/foreground` token added for semantic correctness on Destructive variants.
- **Phase 4 QA pass**: not started — runs after token-spec doc + remaining components in PSY-650/651/652/653 scope are decided.
- **Token spec deliverable** (`docs/features/design-system.md`): not yet drafted — gates PSY-647..653.
- **G1 audit (last run 2026-05-24, post-Sheet)**: 0 mismatches across each new-component page (scoped audits).
- **Dark mode contrast audit (2026-05-24)**: full report — text pairs all AA or better, focus rings + tab underline pass 3:1 non-text; resting hairlines (`border`, `input`) intentionally below 3:1 per editorial direction; light-mode `primary/foreground` on `primary` is AA-large only (3.71) — pre-existing Button decision.
- **DS library publish**: published 2026-05-24 — Textarea/Checkbox/TabTrigger/Sheet available via `componentKey` from consumer files.

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

`get_metadata` without a `nodeId` only lists the FIRST page (re-confirmed 2026-05-28: returns only `Cover` even on a 10-page file) — don't trust it for full inventory. Use the script above.

**⚠ Multi-page-loop caveat (added 2026-05-28):** the script above loops `setCurrentPageAsync` over every page (lines `for (const page of figma.root.children) { await figma.setCurrentPageAsync(page); … }`), which the current `figma:figma-use` skill explicitly discourages ("call `setCurrentPageAsync` at most once per `use_figma` invocation; never switch pages inside a loop — fan out in parallel instead"). It still *works* and is fine for a quick one-shot inventory of a small file (the DS file is ~10 pages). To follow the rule on larger files, split it: the `pages` / `collections` / `textStyles` / `effectStyles` parts need **no** page switch and run in one call; for the per-page `components` walk, do a first read-only call that returns `figma.root.children` page IDs, then emit N parallel `use_figma` calls (one per page) that each set the page once via `setCurrentPageAsync` and collect that page's `COMPONENT`/`COMPONENT_SET` nodes (use `findAllWithCriteria({ types: [...] })` — far faster than the `findAll` predicate).

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

## Product-design workflow (file `XakQQ0nYGqnt77PrHKO9IE` — separate from DS build)

When a product ticket (e.g. PSY-370 /explore, PSY-823 Collections drawer) needs visual mocks before its impl ticket is filed, do this in the Product Designs file, NOT the DS file. The DS file stays governance-only.

### Read the actual app FIRST (the single most important lesson)

The brief tells you WHAT to put on the page; the existing app tells you HOW. Before drawing anything, read enough of the frontend to know:

1. **The chrome that wraps every page** — `frontend/app/layout.tsx` + `frontend/components/layout/SidebarLayout.tsx` + `TopBar.tsx` + `Sidebar.tsx`. Don't invent custom top nav; pages render inside the existing TopBar+Sidebar shell.
2. **A representative page that follows current conventions** — usually `frontend/app/page.tsx` (homepage) plus one entity page (e.g. `frontend/app/shows/page.tsx`). Note: container width (`max-w-6xl`), section gap (`mb-14`), section header pattern (h2 `text-2xl font-bold tracking-tight` + `View all →` link), card style (`bg-card/50 border border-border/50 rounded-xl p-6`).
3. **The visual idiom** — the app is text-led, dense, "music publication" feel (per Locked design direction §Density). No hero photography on the homepage. No marketing-landing energy. Active nav uses `bg-accent text-accent-foreground` highlight, NOT a primary-color underline.

Skipping this step costs an entire iteration cycle. Caught the hard way 2026-05-23 (PSY-370 v1 mock was a marketing landing with 56px headlines and bg-photo hero — completely off-idiom; full rebuild required after surfacing the gap).

### File conventions (revised 2026-05-27 — page-per-AREA, not page-per-ticket)

- **Single file** for all product mocks (no per-feature files at this team scale).
- **Pages organized by product area, NOT by ticket.** Each area page is a vertical-stack auto-layout container holding labeled ticket sections. Multiple tickets contributing to the same surface stack on one page so cross-ticket patterns (component reuse, copy drift, recurring UX questions, state evolution) are visible at a glance.
- **Pages, in order:**
  - `Cover` — workspace explainer (Figma's auto-generated; stays as page 0).
  - `📑 Index` — curated TOC at page 1. Lists each area page with the tickets that contributed, plus cross-refs to legacy companion pages.
  - **Area pages**, single-word PascalCase names: `Discovery`, `Collections`, `Admin`, … Each is a vertical-stack auto-layout with `[page header, ticket section heading, ticket wrapper, ticket section heading, ticket wrapper, …]`. Section heading format: `PSY-XXX — <short description>`.
  - **Legacy** `PSY-XXX — <Feature> · Decisions & Notes` pages — 4 exist as historical mirrors (PSY-370, PSY-845, PSY-853, PSY-871). Preserved as-is. New work uses the HYBRID Decisions pattern below; do NOT create new companion pages.
- **Why area pages, not per-ticket pages** (revised from the original 2026-05-27 morning convention): the page-per-ticket pattern fragmented work — Collections curation surfaces lived across 4 separate pages (PSY-823, PSY-845, PSY-853, parts of PSY-871) and you couldn't see "what does Collections look like as a whole." Per the 2025/2026 Figma-org research, state iterations and cross-ticket evolution of the same surface conventionally live as stacked frames on one area page, not page-per-iteration. The morning convention was a session invention; this revision corrects it after user pushback. The skill PR (#861) shipped the morning convention; a follow-up PR ships this revision.
- **Page naming**: areas use single-word PascalCase. Section headings inside an area page use `PSY-XXX — <short description>` so the ticket origin is visible while scrolling. Legacy companion pages retain their old `PSY-XXX — <Feature> · Decisions & Notes` names (don't rename — they're historical artifacts).

### Page header convention

**Each area page's header** documents the area:

```
<Area name>
area: <Area>  ·  routes: <routes touched>  ·  contributing tickets: PSY-XXX, PSY-YYY, …
<one paragraph: what surfaces this area covers, how it evolves across tickets>
```

**Each ticket section heading inside an area page** is a single Inter Semi Bold 22pt text node sitting on the workspace bg between sections:

```
PSY-XXX — <short description of what this ticket added>
```

**Each ticket's own wrapper** (the existing inner Page Header convention) still applies, just nested inside the area page's stack rather than at page root:

```
PSY-XXX — <Title>
linear: PSY-XXX  ·  project: <Linear project name>  ·  DS: isfHz0oyFK1ALX19IRGg51
<one-sentence scope>
```

### Decisions documentation pattern — HYBRID (adopted 2026-05-27)

Two surfaces hold a ticket's design decisions:

1. **Short "Locked decisions" sidebar on the ticket's wrapper** — a single primary-bordered card inserted at position 1 of the ticket wrapper (right after the wrapper's Page Header section). Note the wrapper itself now lives nested inside an area page's vertical stack — the sidebar is at position 1 *within the wrapper*, not at the area page level. Title `🔒 Locked decisions` + a compact bulleted list of *just the headline locks* (no rationale, no NOT BUILDING, no open questions). One line per decision. Footer pointer: `↗ Canonical long-form: Linear comment on PSY-XXX` with URL. This is the in-context quick-scan, visible when you scroll to that ticket's section on the area page.

2. **Long-form as a Linear comment on the ticket** — full structure: Decisions locked (with rationale per item), NOT BUILDING (deferred), Open questions (forks awaiting input), Follow-ups & cross-references (related tickets, code paths, prior memory). This is the canonical source of truth.

The previous convention (`PSY-XXX — <Feature> · Decisions & Notes` companion page in Figma) is **legacy**. Existing companion pages stay (PSY-370, PSY-845, PSY-853, PSY-871) — preserve as historical mirrors. The PSY-871 companion was migrated to the hybrid pattern (banner stub points to its canonical Linear comment); apply the same migration retroactively to prior companions if they need an update.

**Why hybrid:** the in-Figma adjacency of decisions to the design is real value (zero-context-switch when looking at the mock); but duplicating the full rationale across Figma + Linear is friction-prone, and Linear is already where the rest of the ticket conversation happens. Hybrid keeps the adjacency for the headline, pushes deep work to its natural home.

**Industry context:** the original Figma-companion pattern was a session invention. Late-2025 / early-2026 research (Notion design-system guides, Specify, Vercel design-engineering posts, zeroheight) confirms the industry default is to push decision docs to Linear / Notion / Confluence and embed Figma frames INTO those docs (not the reverse). Our hybrid is a compromise — preserve a quick-scan in Figma, canonical in Linear.

**Workflow for a new ticket's decisions:**
1. As design surfaces decisions, capture them informally during the session.
2. At lock time, draft the long-form once as a Markdown file (e.g. `/tmp/psy-XXX-decisions.md`).
3. Post as a Linear comment on the ticket via `linear issue comment add PSY-XXX --body-file /tmp/...`. Capture the returned comment URL.
4. Build the short sidebar inside the ticket's wrapper at position 1 (right after the wrapper's own Page Header). Just the headline locks + footer pointer to the Linear comment URL. The wrapper itself lives nested in its area page's vertical stack — the sidebar travels with it.
5. **Do NOT create a separate Figma companion page** for new work. The area page's section heading already names the ticket; the wrapper-internal sidebar covers the in-Figma scan; the Linear ticket carries the canonical decisions doc.

### Workflow steps

1. **Inspect the DS file** to harvest the library keys you'll need. The inventory script in §"Verify state" (run against the DS file key) returns every variable / component / text-style / effect-style key. Keep them as constants in your build scripts.

2. **Read the actual app** per the "Read the actual app FIRST" subsection above.

3. **Append to the appropriate area page** in the Product Designs file (or create a new area page if the work introduces a previously-undesigned product surface area). One `use_figma` call: get the area page's stack container by id, append a `PSY-XXX — <description>` section heading text node, then append the ticket's own wrapper frame (1440-wide, with the standard inner Page Header + scope). Save the wrapper id — subsequent calls will append sub-sections to it. Do NOT create a new page per ticket.

4. **Build sections incrementally** per the `figma-generate-design` skill (one section per call, ~10 logical ops max). Inside each section:
   - Use `figma.variables.importVariableByKeyAsync(<key>)` for colors / spacing / radius (DS library variables — no manual "Add to file" needed in the consumer; see G11).
   - Use `figma.importComponentSetByKeyAsync(<key>)` for components.
   - Use `figma.importStyleByKeyAsync(<key>)` for text + effect styles.
   - For multi-axis component instances, use `instance.setProperties({Variant: 'Outline', Size: 'Medium', [labelKey]: 'Cancel'})` per G10 — exact-name `find()` silently fails.

5. **Screenshot after each section** via `get_screenshot` on the wrapper. Validate visually before moving on.

6. **Capture decisions per the HYBRID pattern** (see §"Decisions documentation pattern" above) — long-form as a Linear comment on the ticket, short sidebar on the design page itself. Each open question from the brief gets an answer + one-sentence rationale in the Linear comment; the sidebar carries only the headline lock.

7. **User checkpoint** — present screenshot, await explicit blessing before filing impl ticket(s).

### When the brief is wrong

The brief is a starting point, not scripture. The mock pass often surfaces that the brief over-specified something speculative (e.g. PSY-370's "trending algorithm" with no users to train on) or under-specified something important (PSY-370's traveler use case → city filter). Capture both kinds in the **Linear comment** (per the hybrid pattern):
- "NOT BUILDING:" sections for over-spec the mock removed.
- "FOLLOW-UP TICKET:" sections for under-spec the mock surfaced — and file those tickets.

Then update the brief inline as part of each impl PR — don't ship a docs-only PR; brief paragraphs decay fastest when they're edited far from the code they describe.

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

### G10. Exact-match variant selection silently fails on multi-axis sets

For a multi-axis component set (e.g., Button with `Variant` × `Size` = 18 combos), child variant names are full coordinates like `Variant=Default, Size=Small`. A helper that does `set.children.find(c => c.name === 'Variant=Outline')` never matches — there's no child named just `Variant=Outline`. The naive fallback (`children[0]`) then ships the wrong variant silently with no error.

**Failure mode caught 2026-05-24** (PSY-823 Create Drawer mock): both footer Buttons rendered orange-filled because `defaultVariantOf(buttonSet, 'Variant=Outline')` fell through. Cancel was supposed to be Outline; both ended up `Variant=Default, Size=Medium` (which happened to be `children[0]`).

**Fix — always use `setProperties` for variant switching:**

```js
const seed = set.children[0]; // any variant as a seed
const instance = seed.createInstance();
instance.setProperties({
  Variant: 'Outline',
  Size: 'Medium',
});
```

For TEXT / BOOLEAN / INSTANCE_SWAP properties (keys carry suffix hashes like `Label#37:0`):

```js
const props = set.componentPropertyDefinitions;
const labelKey = Object.keys(props).find(k => k.startsWith('Label'));
instance.setProperties({
  Variant: 'Outline',
  [labelKey]: 'Cancel',
});
```

Even on single-axis sets, `setProperties` is the cleaner pattern — there's no reason to keep two different instantiation paths in the same script.

**Current PSY DS audit (as of 2026-05-24):**

- Multi-axis (MUST use `setProperties`): **Button** (`Variant` × `Size`).
- Single-axis (`setProperties` cleaner, exact-match works): **Input**, **Textarea**, **Checkbox**, **TabTrigger**, **Badge** (all axis = `State`).
- Non-variant (just `createInstance()`): **Sheet**, **Card** + sub-components.

Full pattern memory with detection script: `~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/pattern_figma_variant_instantiation.md`.

### G11. Library publishing requires the source file to be in a Team Project — Drafts files can't publish

Figma policy (since Oct 2024): the **Publish library** button is hidden when the source file lives in `Drafts`. Only files moved to a Team Project surface the button. Caught 2026-05-23 setting up the Product Designs file the first time — both DS file and Product Designs file were initially in Drafts; "Publish library..." was greyed out in the file-name dropdown and not visible in the Assets-panel Libraries modal until the file was moved.

**Fix:** From the Figma dashboard, right-click the file → **Move to project** → pick a Team Project (create one if none exists). Then re-open the file; "Publish library..." is now active in both locations.

**Plugin API can import by key without `Add to file`:** once a library is published, `figma.importComponentSetByKeyAsync(<key>)` and `figma.variables.importVariableByKeyAsync(<key>)` work from any other file the user has access to — no need to manually enable the library in the consumer file's Libraries modal. The manual "Add to file" UI step is only needed for designers dragging from the Assets panel; programmatic imports succeed against any published key.

**Verifying a library is reachable** before relying on it: `get_libraries(fileKey: <consumer>)` shows what's added/available. But the import-by-key test is more direct — if the import succeeds, you're set, regardless of what `get_libraries` shows.

### G12. Wrapping text + nested-card FILL — two recipes that bit PSY-853/845 mocks

Two related "things default to HUG" surprises in product mocks. Both look identical to a script error from the agent's side (text overflows the wrapper, card sits at minimum width) but are configuration, not bugs.

**Recipe 1 — Wrapping body/paragraph text in auto-layout:**

A `figma.createText()` node defaults to `textAutoResize = 'WIDTH_AND_HEIGHT'` (grows horizontally, no wrapping). Inside an auto-layout parent, this means the text overflows the parent's bounds instead of wrapping. The fix is **both** of these, in this order:

```js
const para = figma.createText();
parent.appendChild(para);
para.fontName = {...}; para.fontSize = 14;
para.characters = '<long paragraph>';
para.textAutoResize = 'HEIGHT';          // wrap, grow vertically
para.layoutSizingHorizontal = 'FILL';    // take parent's width
```

Setting only `textAutoResize = 'HEIGHT'` leaves the text at its current (likely too-narrow) width. Setting only `layoutSizingHorizontal = 'FILL'` does effectively the same (FILL implicitly switches autoresize off WIDTH_AND_HEIGHT to HEIGHT), but being explicit avoids surprise. Apply to every multi-line text node — page-header scopes, card descriptions, decision body copy, tooltip popovers, etc. PSY-853 + PSY-845 both shipped this fix after seeing overflow in the first screenshot pass.

**Recipe 2 — Cards nested inside FILL sections still default to HUG:**

`figma.createAutoLayout(...)` always returns a frame with both axes HUG, regardless of where you append it. A section wrapper set to `FILL` does NOT propagate to the cards inside it. Cards stay HUG'd to their content (typically the widest child text), often rendering at ~25% of the intended width.

```js
const card = figma.createAutoLayout('VERTICAL', {...});
sectionWrapper.appendChild(card);
card.layoutSizingHorizontal = 'FILL';    // REQUIRED — otherwise card is HUG
```

Easy to forget when you're focused on the FILL set on the OUTER wrapper. PSY-845's heuristic card shipped at ~400px wide on the first pass because this was missed; fixed with a one-line edit. When building a section card, set FILL right after appendChild as a reflex.

## Resume protocol (for new agents picking this up cold)

### Track A: DS build (extending the design system)

1. Load this skill (you're here).
2. Load `figma:figma-use` + `figma:figma-generate-library` skills.
3. Load `figma-reference` if this is an inspection-heavy session (read tools, URL parsing, recovery when MCP misbehaves or desktop isn't on the file).
4. Load Figma MCP tool schemas (see Prerequisites §3).
4. Sanity-check auth: `mcp__plugin_figma_figma__whoami`.
5. Run the §"Verify state" inventory script against the **DS file** (`isfHz0oyFK1ALX19IRGg51`). Cross-reference with the Build state in this skill — if they disagree, file state wins.
6. Pick up the next component. Mirror Button + Badge structure.
7. Use **direct paint construction (G1)** for every clone-then-rebind. Re-run the G1 audit after each component.
8. User checkpoint after every component — present screenshot, await explicit approval.

### Track B: Product mock (extending the Product Designs file)

1. Load this skill (you're here).
2. Load `figma:figma-use` + `figma:figma-generate-design` skills.
3. Load `figma-reference` if this is an inspection-heavy session (read tools, URL parsing, recovery when MCP misbehaves or desktop isn't on the file).
4. Load Figma MCP tool schemas (see Prerequisites §3).
4. Sanity-check auth: `mcp__plugin_figma_figma__whoami`.
5. Run the §"Verify state" inventory script against the **DS file** to harvest library keys.
6. **Read the actual app** per "Product-design workflow → Read the actual app FIRST." Skipping this step costs an iteration.
7. Open the **Product Designs file** (`XakQQ0nYGqnt77PrHKO9IE`); check existing pages so your naming matches the convention (`<Feature ref> (PSY-XXX)` / `<Feature ref> — Decisions & Notes (PSY-XXX)`).
8. Build the new feature page following the Product-design workflow steps.
9. User checkpoint after each major section + at the end before filing tickets.

## Updating this skill

When new gotchas are discovered:
- Add to the Gotchas section (G9, G10, ...) — keep priority ordering.
- If the gotcha is a project-specific *pattern* worth indexing, also write a `pattern_*.md` in the user-level auto-memory dir and link it via `[[name]]` here.
- Bump the "as of YYYY-MM-DD" date in §"Build state" when state changes materially.
