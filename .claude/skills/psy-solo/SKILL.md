---
name: psy-solo
description: Work a single Psychic Homily Linear ticket end-to-end serially in the main worktree. Use when the user provides a single PSY-XXX ticket and wants the full ship workflow (read → plan with AskUserQuestion on spikes → implement → typecheck + scoped tests → /simplify → optional screenshots → file follow-ups → PR) without parallel-worktree overhead. Includes the UI-screenshot pattern (dev stack + agent-browser + gh draft-release upload) for tickets with user-facing changes.
argument-hint: "PSY-XXX [+ context like 'next' or 'merged, next']"
---

# psy-solo: single-ticket end-to-end ship workflow

Encodes the serial workflow for taking ONE PSY ticket from "In Progress" to a merge-ready PR. Built on top of `psy-ticket` (which owns ticket *creation*) and is the lighter-weight sibling of `psy-dispatch` (which owns batched parallel execution).

## When this skill fires

- User points at a single ticket: "Let's do PSY-657", "Next is PSY-656", "Pick up PSY-XXX"
- User accepts a hand-off pointing at a single next ticket: "merged, next" after a queued list has been established (a la the May 2026 Entity Pages Density Rollout sweep)
- A small one-off improvement the user wants worked serially with full PR ceremony (Plan → impl → tests → /simplify → PR)

Do NOT use for:
- A batch of 2+ tickets — that's `psy-dispatch` (parallel worktrees, one agent per ticket).
- Ticket *creation* — that's `psy-ticket`.
- Generic Linear queries — that's `linear-cli`.
- A throwaway debug session where the user is pairing inline and doesn't want a PR.

## Prerequisites

```bash
linear --version                  # 1.11.x verified
gh --version                      # PR creation + draft-release asset hosting
git rev-parse --show-toplevel     # should resolve to the main worktree, not a `.claude/worktrees/*` path
test -f .linear.toml && echo ok   # team_id=PSY pinned
which agent-browser               # if any UI screenshots are planned
```

## The non-negotiables

1. **One issue = one PR.** Branch name `PSY-{N}/{kebab-description}`. PR body includes `Closes PSY-{N}`.
2. **Pull latest main BEFORE starting.** `git -C <repo> checkout main && git pull --ff-only origin main`. Stale main inherits stale-fallout CI from anything that merged in between (see psy-dispatch anti-pattern catalog for the canonical incident).
3. **Surface ambiguity via `AskUserQuestion` BEFORE writing code.** When the ticket has a design fork (Option A vs B, taxonomy/threshold/UX choice not already decided), batch the questions into a single `AskUserQuestion` call. See `feedback_no_speculative_implementation.md` and `feedback_plan_mode_questions_first.md`. Do not guess — file a follow-up rather than implementing speculative scope.
4. **`/simplify` AND relevant local tests run before push; failure blocks push.** Same rule as `psy-dispatch`. Never push past a failing local test, even one you believe is pre-existing on main — escalate to the user instead of triaging via GitHub CI. See `feedback_simplify_before_pr.md`.
5. **Never mark Done in Linear.** Transition to "In Progress" on start; the user moves it to Done on merge.
6. **Never merge the PR yourself.** Push and open the PR; the user merges.
7. **Document deferred scope explicitly in the PR body.** If the implementation Q&A produced a "skip this and file a follow-up" decision, link the follow-up ticket(s) in a `## Deferred` section.
8. **For UI changes: capture screenshots when feasible.** UI tickets benefit from rendered visual evidence in the PR. Use the [Screenshot workflow](#phase-6-screenshots-ui-tickets-only) — skip for backend-only / docs-only / config-only tickets (note "no UI surface" in the test plan instead).

## Workflow

The phases below are how a single PSY ticket goes from a user pointer to a merge-ready PR. Phases 1–5 + 7–8 apply to every ticket; phase 6 (screenshots) applies to tickets that change the rendered UI.

### Phase 1: Pre-flight + branch

```bash
git -C <repo> checkout main && git -C <repo> pull --ff-only origin main
git -C <repo> checkout -b PSY-{N}/{kebab-short-description}
linear issue update PSY-{N} --state "In Progress"   # case-sensitive; "In Progress" with the space
linear issue view PSY-{N}                            # read description, AC, open questions
```

Run the four commands roughly in parallel where they don't depend on each other (pull + checkout + state-update + view can be batched). If `pull --ff-only` fails with `"Diverging branches"`, see psy-dispatch's "Side-branch checkout recovery" — most commonly local main has a stash-WIP commit ahead of origin; pause and ask.

### Phase 2: Read + plan + surface ambiguity

Read the ticket + the immediate code surfaces it points at + the canonical precedent file (e.g. `ArtistDetail.tsx` for entity-page density work). Use `Agent` with `subagent_type: "Explore"` for any broader codebase search.

If the ticket has spike items / open questions / forks not pre-decided, **batch them into a single `AskUserQuestion` call BEFORE writing any code**. Suffix the recommended option with `(Recommended)` and provide one-sentence trade-off `description`s. Don't ask about anything you can determine from the code (file paths, type signatures, function names).

Common spike shapes:
- Subsystem gaps that block a header bracket — e.g. `[Notify me]` for festival requires adding `festival` to `NotifyEntityType` (backend + frontend). User picks "skip + file follow-up" vs "expand scope".
- Visibility logic for new affordances — always-visible-to-auth vs only-when-empty.
- Inclusion / ordering of header BracketLinks against the canonical order.

If a follow-up ticket comes out of phase 2 (user says "skip + file follow-up"), draft the follow-up's description into `/tmp/psy-{N}-followup-<topic>.md` now and file it via `linear issue create` after the PR opens (or now, if it'd be lost otherwise).

### Phase 3: Implement

Standard implementation. Notes specific to this workflow:

- **Verify CWD before editing.** `git rev-parse --show-toplevel` should resolve to the main worktree root, not a worktree under `.claude/worktrees/*`. The single-ticket flow runs in the main repo by design; if `pwd` shifted into a stale worktree from a prior `psy-dispatch` session, `Edit`/`Write` would land in the wrong place.
- **Reuse shared primitives instead of inventing.** For entity-page work: `BracketLink`, `SectionHeader`, `StatsList`, `DenseTable`, `EntityDetailLayout` (flat mode), `AddTagDialog`, `FollowButton(variant="bracket")`, `AddToCollectionButton(variant="bracket")`, `NotifyMeButton(variant="bracket")`, `EntityCollections` (self-hides), `EntityTagList` (self-hides post-PSY-654). The artist page (post-PSY-641/644/645) is the canonical precedent for the linkbox + flat-sections shape.
- **Watch for latent truthy-but-empty bugs at boundaries.** Empty objects (`social: {}`), empty arrays (`venues: []`), and zero counts (`venue_count: 0`) are truthy in JS. When gating a section on optional API data, use shape-aware checks (e.g. `Object.values(social).some(v => !!v)`) instead of `!!social`. Caught in PSY-657: `social: {}` was making an empty Links wrapper render on VIVA PHX 2026.

### Phase 4: Test

Run all tests relevant to the diff. Failure blocks push (rule 4). Order matters:

```bash
cd frontend && bun run typecheck                          # always for any frontend touch
cd frontend && bun run test:run features/<scope>          # scoped vitest (e.g. features/festivals)

# Backend touched? Build BEFORE test (catches whole-graph compile errors):
cd backend && go build ./...
cd backend && go test ./internal/services/<pkg>/...       # scoped test, or `./...` for large diffs

# Modified anything under frontend/e2e/? Run that spec.
# E2E global-setup requires :8080 to be free — STOP and surface if user's dev backend is on it.
```

The actual scoped frontend runner is `bun run test:run <path>`, NOT `bun run test:unit`. The latter doesn't exist; verify scripts via `package.json` if uncertain.

If any test fails — even one you believe is pre-existing on main — STOP, report the failing test + command + one-sentence hypothesis, and let the user decide between (a) fixing inline first, (b) skipping, (c) accepting. Do NOT push and hope GitHub CI sorts it. See `feedback_simplify_before_pr.md`.

### Phase 5: /simplify

Invoke `Skill` with `skill: "simplify"`. The simplify skill spawns 3 parallel reviewer agents (reuse / quality / efficiency) against your diff. Aggregate their findings; fix the actionable ones directly. For findings that are pre-existing peer-wide gaps (e.g. inline `<h2>` headings vs `SectionHeader` primitive when the peer pages all use inline) — note in the PR body, do NOT expand scope.

If `/simplify` produced code changes, re-run the relevant test commands from phase 4.

### Phase 6: Screenshots (UI tickets only)

Skip this phase entirely for backend-only / docs-only / config-only tickets — note `"no UI surface, screenshots skipped"` in the test plan.

For UI tickets, the goal is one or two screenshots embedded in the PR body so reviewers can see the rendered change without a local checkout.

**6a. Start the dev stack as background processes.**

```bash
cd backend && go run cmd/server/main.go                                    # run_in_background: true
cd frontend && NODE_OPTIONS="--max-old-space-size=8192" bun run dev        # run_in_background: true
```

Backend takes ~5–10s to bind :8080; frontend Next.js takes ~2–5s for the dev server but the first page-render is slower (Turbopack first-compile). Wait briefly OR `curl -sS http://localhost:8080/<entity-list-endpoint>` until you get a response.

**`NODE_OPTIONS=--max-old-space-size=8192` is not optional for audit-style sessions.** Default Node heap (~4GB) is enough for a single page-screenshot pass, but Turbopack's incremental compile cache OOMs (`FATAL ERROR: Reached heap limit`) when an audit navigates across 4+ heavy pages in succession (caught: May 16 post-shipped-UI audit — frontend crashed twice mid-sweep). 8GB is safe; 6GB also works.

If the user has a dev backend already running on :8080, the second `go run` will fail with `bind: address already in use` — ask the user before killing the existing one.

**6b. Find a real entity to navigate to.** The local dev DB usually has limited seed data. Query the relevant list endpoint to find a populated entity:

```bash
curl -sS "http://localhost:8080/<entities>?limit=5" | python3 -m json.tool | head -40
```

Pick one with rich AND sparse aspects so the screenshot exercises both render paths (populated lineup + empty venues, populated tags + empty description, etc.). If only one entity exists, that's fine — describe what the screenshot does and doesn't cover in the PR body.

**6c. Capture via `agent-browser`, NOT `chrome-devtools` MCP.** The chrome-devtools MCP requires Chrome / Chrome Beta running with `--remote-debugging-port=9222` on the DEFAULT user-data-dir path — using a custom `--user-data-dir` breaks the MCP's profile lookup. `agent-browser` manages its own browser binary and works first-try.

```bash
agent-browser open http://localhost:3000/<entity>/<slug>
agent-browser wait 1500                                     # let hydration settle
agent-browser screenshot /tmp/psy-{N}-<short>.png            # viewport-only (top of page)
agent-browser screenshot --full /tmp/psy-{N}-<short>-full.png   # full page (all main-column sections)
```

**Prefer separate sequential foreground `agent-browser` calls over chained `open && sleep && screenshot && eval` in a single background bash.** The chained background pattern repeatedly returned ambiguous status (background-task completion with empty output, or the eval racing against navigation and erroring `Execution context was destroyed`). Separate foreground calls are slower per step but every step has visible result/error feedback, so you can react to a failure instead of guessing. Caught: May 16 audit — at least 4 chained-background calls hung or returned empty before I switched to foreground-sequential.

**`agent-browser navigate reload` doesn't work** — the CLI interprets `reload` as a URL and tries to navigate to `https://reload/`. To reload after a code change (re-hydration of changed component, etc.), re-issue `agent-browser open <same-url>`. Caught: PSY-656 manual repro after the year-bug fix.

Read the PNG back with the `Read` tool to verify visually before uploading — every once in a while, render is missing a section because hydration hadn't completed, or the dev server hit a runtime error you didn't see in the log. Heavy artist pages can need 12–15s for full hydration (multiple parallel data fetches — shows, discography, similar, labels, festival appearances). If the first screenshot looks blank or partial, wait another 5–8s and re-capture before assuming a render bug.

**6d. Sanity-check rendered structure via JS eval.** Small / scaled-down sections (e.g. sidebar `StatsList`) can be hard to read in a full-page PNG. Confirm they rendered correctly by querying the DOM directly:

```bash
agent-browser eval "Array.from(document.querySelectorAll('aside')).map((a,i) => 'aside['+i+']: '+a.innerText.replace(/\n/g,' | ').slice(0,300)).join('\n---\n')"
```

This caught PSY-657's "is the sidebar StatsList actually rendering?" doubt — DOM showed `STATISTICS | Artists | 43 | Venues | 0` even though the PNG made it look empty.

**Dialog-visibility checks: do NOT use `offsetParent !== null` for Radix Portal dialogs.** Radix portals dialog content into `<body>` via `Portal`; `offsetParent` returns `null` for those nodes in some hydration states, producing false-negative "dialog is closed" reads. Use `data-state="open"` attribute (set by Radix) or `getComputedStyle(d).display !== 'none'` instead:

```bash
# Correct dialog-open check:
agent-browser eval "
const d = Array.from(document.querySelectorAll('[role=\"dialog\"]')).find(el => el.innerText.includes('<dialog title fragment>'));
({ dataState: d?.getAttribute('data-state'), display: d ? getComputedStyle(d).display : null })
"
```

Caught: May 16 post-shipped-UI audit — `offsetParent` false-negative made the Graph Dialog look like it never auto-opened from `#graph` URL; subsequent screenshot confirmed it DID open (the bug was on close, not on open).

**6e. Auth-state screenshots are usually skippable.** Unauthenticated screenshots show the structural changes (flat layout, hide-when-empty, sidebar shape, public BracketLinks like `[Follow]`). Auth-only brackets (`[Edit | Suggest edit]`, `[Suggest description]`, `[Add tag]`, `[Report]`) are visible in the diff and can be described in the PR body. Skip the auth login flow unless the reviewer needs to see the rendered auth-only state.

**6f. Upload via `gh release create --draft` for image hosting.** GitHub markdown can't embed local files or base64 PNGs, and `gh` has no direct image-upload API for PRs. The reliable path: create a DRAFT release with the PNGs as assets and embed the asset download URLs.

```bash
gh release create psy-{N}-screenshots \
  --draft \
  --notes "Screenshot assets for PSY-{N} PR. Draft — not visible on Releases page." \
  /tmp/psy-{N}-*.png

# Get the URLs for the PR body:
gh release view psy-{N}-screenshots --json assets --jq '.assets[].url'
```

Draft releases:
- Do NOT appear on the public Releases page (only repo admins see them, in a "Drafts" section).
- Their assets get auto-generated `untagged-<hash>` URLs like `https://github.com/owner/repo/releases/download/untagged-cdfc460b25382f07bab3/file.png`. Ugly but functional.
- Asset URLs render embedded in PR markdown for any user authed to the repo. Private repos: only authed viewers see the images.
- The draft stays around forever unless deleted; that's fine for retroactive PR review.

**6g. Tear down dev servers when screenshots are done** (Phase 9). Leaving them running blocks the next `psy-solo` invocation that needs :8080 / :3000.

### Phase 7: File follow-ups

Any deferred scope from phase 2's Q&A becomes a Linear ticket NOW (before opening the PR, so the PR body can link them):

```bash
linear issue create \
  --team PSY \
  --title "<short title>" \
  --label <relevant> --label Improvement \
  --priority 3 \
  --project "<active project name>" \
  --description-file /tmp/psy-{N}-followup-<topic>.md \
  --no-interactive
```

Description files should explain: the problem, the proposed change (backend / frontend / both), open questions, acceptance criteria, and a `## Source` line citing the parent ticket. See PSY-660 / PSY-661 / PSY-662 from the May 2026 Entity Pages Density Rollout for canonical examples.

**Capture the filed PSY IDs** — you'll substitute them into the PR body's `## Deferred` section in phase 7.5. Filing in phase 7 BEFORE phase 7.5 / 7.6 means the PR body references real IDs (not `PSY-XXX` placeholders that `/psy-self-review` would then flag — caught: PSY-656 self-review warned about an unsubstituted placeholder because filing happened too late).

### Phase 7.5: Draft PR body to /tmp/

Write the planned PR body to `/tmp/psy-{N}-pr-body.md`. Use the [phase 8 template](#phase-8-commit--push--open-pr) below — substitute the real PSY IDs from phase 7 for any `PSY-{M}` placeholders. This is a real artifact (not a mental draft) because `/psy-self-review` (next phase) needs to read it.

### Phase 7.6: /psy-self-review

Invoke `Skill` with `skill: "psy-self-review"`, passing the `/tmp/psy-{N}-pr-body.md` path as arg. It spawns 3 parallel reviewer sub-agents that check the drafted PR body against session evidence:
- Agent 1: every `[x]` test-plan item has a matching command / screenshot / test run in the session log
- Agent 2: every behavior change in the diff is claimed-or-disclaimed in the PR body
- Agent 3: convention + asymmetry traps (unauth fallback symmetry, truthy-empty gate shapes, dialog open/close URL-hash symmetry, year-shape numerics passed to `StatsList` without `String()` wrap, unresolved `PSY-XXX` placeholders)

**BLOCKING findings** (claims with no evidence) STOP the push — fix by re-doing the verification OR downgrading the `[x]` to `[ ]` with a "deferred manual repro" note. **WARNING / NIT findings** are agent's judgment call (warnings usually require a PR-body patch; nits usually require a small disclosure line).

The skill is honest about scope (can't verify what a screenshot SHOWS, only that it exists; cannot audit PR title since it's passed via `--title` separately from the body; spot-Read PNGs yourself for load-bearing claims).

Required for any PR with a `## Test plan` containing `[x]` items. Skip only for backend-only / docs-only / config-only PRs where `/simplify` already covers the audit surface.

Born out of the May 16–17 retro: PSY-658 shipped with an unverified `[x]` claim that caught a real bug post-merge (PSY-663). This phase exists to prevent that recurring. See `.claude/skills/psy-self-review/SKILL.md`.

### Phase 8: Commit + push + open PR

The PR body is the file you wrote in phase 7.5 and refined in phase 7.6 — use `--body-file`, not an inline heredoc.

```bash
git -C <repo> add <specific files>                # NOT `git add .` — never accidentally add untracked
git -C <repo> commit -m "PSY-{N}: <imperative summary>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
git -C <repo> push -u origin PSY-{N}/<branch>
gh pr create --title "PSY-{N}: <under-70-char summary>" --body-file /tmp/psy-{N}-pr-body.md
```

#### PR body template (use as starting point for the phase 7.5 draft)

```markdown
Closes PSY-{N}.

## Summary
- <bullet 1>
- <bullet 2>

## Deferred (per pre-implementation Q&A)
- **<what was skipped>** — <one-sentence reason>. Filed **PSY-{M}** to address.

## Heads up from `/simplify`
<only include this section if /simplify surfaced a non-blocking concern worth flagging — e.g. an efficiency regression that's intrinsic to the design. Omit if /simplify was clean.>

## Test plan
- [x] `bun run typecheck` — clean
- [x] `bun run test:run features/<scope>` — N/N passing
- [x] `/simplify` — <outcome>
- [x] `/psy-self-review` — <outcome: clean / N findings addressed>
- [x] Manual repro against local dev stack with <entity>: <one-sentence description of what was visually verified>

## Screenshots
<embed the asset URLs from phase 6f, with one-sentence captions describing what each shows>

![<alt>](<asset-url>)

## Coverage gaps

Honest disclosure of what the screenshots / manual repro do NOT cover:

- **Rich-data paths not exercised** (dev DB is sparse — PSY-665 will unblock): <list which optional fields weren't populated on the entity tested>
- **`[Add tag]` → `AddTagDialog` open / submit / close cycle not exercised** (gated on auth; dialog primitive shared across all entity pages).
- **`canEditDirectly ? 'Edit' : 'Suggest edit'` label variant not visually verified** (trust-tier conditional; only unauth was exercised, so neither label was rendered).
- **Authenticated header brackets not visually verified**: `[Edit | Suggest edit]`, `[Suggest description]` (when desc empty), `[Add tag]`, `[Report]` only render for authenticated viewers. Skipped the auth login flow per psy-solo convention; the brackets exist in the diff at <file:line>.
- <add any UI-ticket-specific gaps here, e.g. multi-bucket grouping not observable because dev data has only one bucket type>

EOF
```

The 3 bracket-related lines under `## Coverage gaps` are standard for any UI ticket using the header-linkbox + AddTagDialog pattern. Include them by default; remove only if the specific PR doesn't touch them.

PR title under 70 chars (verify manually — `/psy-self-review` cannot audit titles passed via `--title`). Body MUST include `Closes PSY-{N}`. If `/simplify` was clean, drop the `## Heads up` section. For non-UI tickets, drop `## Screenshots` and add a Manual repro line in the Test plan ("no UI surface" or curl-against-backend response).

### Phase 9: Cleanup

```bash
kill <backend-PID> <frontend-PID>                 # if dev servers were started in phase 6a
lsof -i :8080 -i :3000                            # verify no listeners (CLOSED entries from other apps are fine)
```

PIDs come from the original `run_in_background` task output (each background Bash result includes the PID). If you can't find them, `pgrep -fl "go run cmd/server" -fl "next dev"`.

Do NOT delete the draft release after the PR is open — the asset URLs depend on it persisting. The user (or you, post-merge cleanup) can sweep old draft releases periodically.

## Anti-patterns

- **Treating tool failures as a reason to declare done.** `agent-browser` hangs, Turbopack OOMs, MCP errors out — these are recoverable, not stop conditions. Recover the tool (restart with bigger heap, split chained calls into foreground sequential, switch MCPs), investigate the root cause if it's blocking, continue the work. Caught May 16: I almost wrapped a multi-page audit prematurely when frontend OOMed twice + agent-browser background calls hung repeatedly. User correction: "let's not get lazy. If the audit encountered hangs and a crash, let's pick up where we left off after cleaning up what was broken." Recovery (restart frontend with `--max-old-space-size=8192`, switch to foreground sequential calls) took ~5 minutes and unblocked the rest of the audit, which then caught the bug that prompted filing PSY-664.
- **Starting frontend without `NODE_OPTIONS=--max-old-space-size=8192` for an audit-style session.** Default ~4GB heap OOMs under Turbopack's incremental cache after 4+ page navigations. Caught twice on May 16 — frontend crashed with `FATAL ERROR: Reached heap limit` mid-audit; restart with 8GB heap unblocks. Single-page screenshot is fine without the override.
- **Treating `offsetParent !== null` as a reliable dialog-visibility check.** Radix Portal dialogs return `null` for `offsetParent` in some hydration states, producing false-negative "closed" reads. Use `data-state` attribute or `getComputedStyle().display`. Caught May 16: nearly filed a false "dialog doesn't auto-open from URL hash" bug before the screenshot proved otherwise.
- **Skipping `AskUserQuestion` because "the audit doc says X"**. Audit / research docs are point-in-time; re-verify against current code before relying. PSY-657 caught a real bug here: the audit suggested `[Notify me]` for empty festival lineups, but `NotifyEntityType` doesn't include `festival` — surfaced as a spike question rather than as silent scope expansion.
- **Pushing past failing local tests.** Same rule + same incident catalog as `psy-dispatch` rule 3. Do not push and hope GitHub CI sorts it.
- **Using chrome-devtools MCP without verifying Chrome's `DevToolsActivePort` file is at the default profile path.** The MCP looks at `~/Library/Application Support/Google/Chrome Beta/DevToolsActivePort` (or the equivalent for vanilla Chrome). Launching Chrome with `--user-data-dir=/tmp/...` puts the file in a custom location the MCP can't see. `agent-browser` sidesteps the whole issue; prefer it for one-off screenshot capture.
- **Embedding `file:///tmp/...` paths in the PR body.** GitHub markdown doesn't fetch local file URIs. Use `gh release create --draft` to host the PNG and embed the asset URL.
- **Asking the user to drag-drop screenshots into the PR after opening.** Works once, but breaks the "PR is review-ready when opened" contract. The draft-release upload is automatable and reliable; prefer it.
- **Committing untracked files via `git add .`.** Always stage specific paths. The single-ticket flow operates in the main worktree, which usually has stray untracked files from session-scope work (skill drafts, ad-hoc notes, screenshots in `/tmp` accidentally moved). `git add -A` or `.` will sweep them in.
- **Forgetting to tear down dev servers.** Next session's `psy-solo` for the same ticket queue will hit `bind: address already in use`. Always run phase 9.
- **Drafting the PR body without a `## Deferred` section when scope was actually deferred.** The reviewer needs to see WHY the obvious-next-bracket / obvious-next-section / obvious-next-handler isn't included. Hiding it sets a precedent for follow-up tickets to be forgotten.
- **Truthy-but-empty data gating bugs at API boundaries.** Empty objects, empty arrays, zero counts are all truthy in JS. Always use shape-aware checks (`Object.values(x).some(...)`, `.length > 0`, `!= null && ...`) when gating sections on optional API data. Caught in PSY-657 on `social: {}`.
- **Claiming PR test-plan items you didn't actually verify.** The PSY-658 PR test plan listed `unauthenticated viewer (only [Add to collection] visible in linkbox)` without a screenshot to back it. Post-shipped-UI audit (PSY-663) caught that `AddToCollectionButton.tsx:99` returns null for unauth — the linkbox was empty, not single-bracket as claimed. If you can't visually verify a Test plan item before push, mark it `[ ]` (unchecked) with a brief "deferred manual repro" note. `[x]` is a statement that you verified it.
- **Asymmetric `if (!isAuthenticated) return null` patterns across peer shared components.** `FollowButton` and `NotifyMeButton` (bracket variants) RENDER for unauth and redirect to `/auth` on click; `AddToCollectionButton` returns null entirely. When auditing a new shared-component variant, check peer components for the convention before deciding the unauth fallback shape. Caught: PSY-663.

## Related skills and memories

- **`psy-dispatch`** — parallel-worktree batch execution. Use when 2+ tickets need to ship; `psy-solo` is for single tickets where worktree overhead would be friction.
- **`/psy-self-review`** — invoked at phase 7.6 between follow-up filing (phases 7 / 7.5) and `git push` (phase 8). Sub-agent audits the draft PR body against session evidence; BLOCKING finding (unverified `[x]` claim) stops the push.
- **`/psy-audit` (planned, post-PSY-656)** — multi-page post-shipped UI audit pattern (sweep N merged tickets via screenshots + DOM-eval, file follow-ups, post project-update). Different scope from `psy-solo` (retrospective sweep vs forward per-ticket). Will be drafted after PSY-656 validates the audit cadence is genuinely reusable. May 16 audit was the first instance: caught PSY-663 + PSY-664 in ~30 minutes.
- **`psy-ticket`** — ticket creation; pair with phase 7 to file the follow-ups this skill identifies.
- **`linear-cli`** — generic Linear surface; drop down to it if `linear issue` lacks a flag.
- **`simplify`** — invoked in phase 5; spawns 3 parallel reviewer agents (reuse / quality / efficiency) against the diff.
- **`agent-browser` CLI** — `which agent-browser` to confirm install; pre-installed in this dev environment. Reliable Chrome automation that doesn't depend on a running Chrome instance.
- `feedback_simplify_before_pr.md` — non-negotiable rule 4 above.
- `feedback_no_speculative_implementation.md` — non-negotiable rule 3 above (ask, don't guess).
- `feedback_plan_mode_questions_first.md` — surface forks via `AskUserQuestion` BEFORE implementation.
- `feedback_verify_before_push.md` — manual repro + screenshot in phase 6 verifies feature-correctness; tests verify only code-correctness.

## Project pointers

- Active project (May 2026): `"Entity Pages Density Rollout — May 2026"` Linear project; queue is PSY-655 (shipped) → PSY-654 (shipped) → PSY-658 (shipped) → PSY-657 (shipped) → PSY-656 (queued).
- Canonical precedent files: `frontend/features/artists/components/ArtistDetail.tsx` (post-PSY-641/644/645) for header linkbox + flat layout; `frontend/features/artists/components/ArtistShowsList.tsx` for the two-section shows pattern.
- Shared primitives: `frontend/components/shared/{BracketLink,SectionHeader,StatsList,DenseTable,EntityDetailLayout,EntityHeader,FollowButton,AddToCollectionButton}.tsx`.
- Audit source: `docs/research/entity-page-empty-state-audit.md` (PSY-643 deliverable; treat counts/sites as point-in-time and re-verify).
- Linear CLI essentials in repo root `CLAUDE.md`.
