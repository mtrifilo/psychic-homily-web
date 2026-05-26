---
name: figma-reference
description: Reference for reading from Figma reliably + recovering when the Figma MCP misbehaves (server disconnect, desktop-app-on-wrong-file silent failures, oversized metadata responses). Workspace-agnostic. Pairs with the `figma:*` plugin skills (which cover WRITE workflows — `figma-use`, `figma-generate-design`, `figma-generate-library`, `figma-code-connect`) and with `psy-design-system` (PSY-specific file keys + locked decisions). Use whenever a session needs to inspect a Figma file before designing/writing to it, or to debug an MCP read that returns unexpectedly empty results.
argument-hint: "[read|recovery|url-parse]"
---

# figma-reference: reading Figma + recovering when MCP misbehaves

Workspace-agnostic reference for the Figma MCP **READ surface** plus the non-obvious gotchas that have repeatedly bitten agent sessions. The `figma:*` plugin skills cover WRITE workflows (mandatory prerequisites before `use_figma` / `generate_figma_design` / `create_new_file` / `generate_diagram` tool calls). This skill covers READ + RECOVERY + URL PARSING — the patterns underneath them all.

## When to load this skill

- You need to **inspect a Figma file** before designing or writing to it (e.g. confirm a frame exists, find a node ID, screenshot a state).
- The MCP `get_metadata` call returned **only the "Cover" page** when you expected more pages — the desktop-app-on-wrong-file silent-failure pattern. See the gotcha below.
- The MCP returned **a "result exceeds maximum allowed tokens"** error and you need the file-extraction pattern.
- You need to **parse a Figma URL** into `fileKey` + `nodeId`.
- The MCP server **disconnected mid-session** and you need recovery options.

Do NOT load this skill for:
- WRITES to a Figma file — that's `figma:figma-use` (plus `figma:figma-generate-library` / `figma-generate-design` per the workflow).
- PSY-specific design-system work (file keys, locked direction, build state) — that's `psy-design-system`. It loads `figma-use` + `figma-generate-library` on top of this skill's surface.

## Read tools — when to use which

The MCP exposes three read-shaped tools. They overlap in purpose; pick by what you actually need:

| Tool | Returns | Use when |
|---|---|---|
| `mcp__plugin_figma_figma__get_metadata` | XML structure (no design tokens, no code) | You need to find a node by name, or enumerate a frame's children. Cheap; defaults to listing top-level pages when `nodeId` is omitted. |
| `mcp__plugin_figma_figma__get_screenshot` | Short-lived PNG URL + dimensions | You want to visually confirm a frame's layout, or capture an asset for a PR body. Pass `maxDimension` (default 1024) for resolution. |
| `mcp__plugin_figma_figma__get_design_context` | Reference code + screenshot + asset URLs | You're translating Figma → code (the canonical design-to-code workflow). |

Decision tree:

```
Need to FIND a node?              → get_metadata (with nodeId of parent)
Need to SEE a node?               → get_screenshot
Need to IMPLEMENT a node as code? → get_design_context
Don't know which?                 → get_metadata (cheapest; reveals structure)
```

## Critical gotcha — Figma desktop app must be open on the target file

The MCP requires the **Figma desktop app** to be running AND have the target file **currently open** (active tab). When the desktop is on a *different* file, the MCP silently returns a stub: usually just the "Cover" page with no children. **No error, no warning** — the API succeeds with a misleading "this file is empty" shape.

Symptoms:
- `get_metadata` (no nodeId) returns one page named "Cover" and nothing else.
- `get_metadata` on `0:1` (the Cover page) shows only file-description text, no design content.
- `get_screenshot` on a node ID you KNOW exists returns "Node not found" or an empty render.

Recovery:
1. Ask the user to **open the file in Figma desktop and bring it to the foreground**. The file URL (`https://www.figma.com/design/<fileKey>/...`) is the signal — they can paste it into the desktop app's File → Open URL.
2. After they confirm the file is open, retry the MCP call.
3. The desktop app's "currently focused file" determines which file MCP can read — switching desktop tabs changes which file the next MCP call resolves against.

Examples of when this bit us:
- PSY-823 session: agent assumed file was reachable, spent multiple turns probing before discovering desktop was closed. Lost ~10 minutes.
- PSY-824 session: same pattern, recognized faster but still needed the user to confirm desktop state.

**Always ask the user to confirm the file is open in desktop before any READ if the first call returns suspiciously empty results.**

## URL parsing

Figma URLs encode the file key + an optional node ID:

```
https://www.figma.com/design/<fileKey>/<fileName>?node-id=<id1>-<id2>
                            ^^^^^^^^^                      ^^^^^^^^^
                            file key                       node ID (URL form)
```

- **fileKey** is the path segment after `/design/` (alphanumeric, no dashes inside; the dashes in the file *name* are part of the URL-encoded name, not the key).
- **nodeId** in the URL uses `-` as separator (`node-id=44-2`), but the MCP API expects `:` (`44:2`). **Convert dash → colon** before passing to any MCP tool.
- Branch URLs: `figma.com/design/<fileKey>/branch/<branchKey>/<fileName>` — use the `<branchKey>` as the fileKey for the API call.

Example: URL `https://www.figma.com/design/XakQQ0nYGqnt77PrHKO9IE/Psychic-Homily-Product-Designs?node-id=44-2`
- fileKey: `XakQQ0nYGqnt77PrHKO9IE`
- nodeId: `44:2` (after dash→colon conversion)

## Metadata-too-large pattern

`get_metadata` on a large frame can return >100k tokens. The MCP harness then saves the result to a file under `~/.claude/projects/<project>/<session>/tool-results/` and surfaces an error pointing at the saved file. Extract what you need with `jq`:

```bash
# The MCP error tells you the file path. Probe structure first:
jq 'type, length, keys?' /path/to/saved-file.txt

# Extract just the top-level frame IDs + names:
jq -r '.[].text' /path/to/saved-file.txt \
  | grep -oE 'id="[0-9]+:[0-9]+" name="[^"]*"' \
  | head -40

# Or extract a specific subtree by node ID:
jq -r '.[].text' /path/to/saved-file.txt \
  | grep -A 50 'id="44:7"'
```

The MCP error message includes guidance like "For targeted queries (find a value, filter by field): use jq on the file directly." Read the saved file's first line for the schema before jq-piping.

Alternative: pass a more specific `nodeId` (deeper in the tree) so the metadata returned is bounded. Top-level pages of large files almost always exceed the token limit; their immediate children usually fit.

## Screenshot workflow

```typescript
// Pass nodeId in colon form:
get_screenshot({ fileKey: "...", nodeId: "44:7", maxDimension: 1400 })
```

The response includes:
- `image_url` — short-lived (treat as a secret)
- `width` / `height` — rendered PNG dimensions
- `original_width` / `original_height` — pre-clamp node dimensions
- A curl one-liner to download

Download via `curl -o /tmp/<name>.png "<url>"` then `Read` the PNG to verify visually. If the render looks blank or partial, wait a few seconds and re-request — Figma's screenshot service can be slow on the first render of a large node.

`maxDimension` defaults to 1024. Bump to 1400+ for fine-detail inspection; drop to 512 for thumbnails.

## Recovery patterns

**MCP server disconnected mid-session** (e.g. system-reminder says "deferred tools are no longer available"):
- Read-only inspection is blocked until the MCP reconnects.
- Workaround: ask the user to paste a screenshot of the relevant Figma frame into the chat, then `Read` the screenshot image directly.
- For dimensions/structure you can't see in a screenshot, ask the user to copy-paste the relevant layer panel info.

**Screenshot URL expired** (URLs are short-lived):
- Re-call `get_screenshot` to get a fresh URL.
- If the same node renders inconsistently across calls (rare), pass `contentsOnly: true` to isolate the node from overlapping content.

**Asset URL 404 on download**:
- Same fix — short-lived URLs expire. Re-issue the MCP call.

**get_metadata returns wrong/stale content after a file edit**:
- The user may need to refresh the file in desktop (Cmd+R or close+reopen the tab). Figma's MCP reads from the desktop's local cache, which can lag.

## File-key + node-ID conventions

Across this project, file keys live in `psy-design-system/SKILL.md`. Don't hard-code them in code (they're identifiers, not constants). Don't paste them into commit messages either — they're stable but not interesting in source history.

When recording a new frame as a reference, save the `(fileKey, nodeId-in-colon-form, label)` triplet so the next session can re-fetch without re-parsing URLs.

## Related skills

- **`figma:figma-use`** — WRITE operations on Figma files via the Plugin API (create/edit/delete nodes, variables, components). MANDATORY before any `use_figma` tool call.
- **`figma:figma-generate-design`** — translating an app page/view/layout INTO Figma. Pair with `figma-use`.
- **`figma:figma-generate-library`** — building a design system in Figma from a codebase (variables, components, variants, theming).
- **`figma:figma-code-connect`** — Code Connect templates mapping Figma components to code snippets.
- **`figma:figma-create-new-file`** / **`figma:figma-generate-diagram`** / **`figma:figma-use-figjam`** / **`figma:figma-use-slides`** — specialized write workflows; mandatory before their corresponding `create_new_file` / `generate_diagram` / FigJam / Slides tool calls.
- **`psy-design-system`** — Psychic Homily PSY-646 design-system context. Loads file keys, locked design direction, accumulated gotchas (G1: bound-paint cache miss, G10: multi-axis variant exact-match silent failure, etc.). Project-specific overlay on `figma:figma-use` + `figma:figma-generate-library`.
