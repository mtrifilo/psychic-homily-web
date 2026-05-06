---
name: psy-dispatch
description: Dispatch a batch of Psychic Homily Linear tickets in parallel git worktrees. Use when the user provides a list of PSY-XXX tickets and asks to "tackle in parallel", "dispatch in worktrees", "batch these tickets", or otherwise work multiple tickets concurrently without blocking each other. One agent per ticket, each in its own isolated worktree, each runs `/simplify` before opening its PR. Resolves ambiguity via AskUserQuestion BEFORE dispatch.
argument-hint: "[PSY-XXX, PSY-YYY, ... or screenshot of Linear list]"
---

# psy-dispatch: parallel-worktree ticket dispatch

Encodes the workflow for taking a batch of PSY tickets and dispatching one parallel-worktree agent per ticket. Built on top of `psy-ticket` (which owns ticket *creation*); this skill owns ticket *execution*.

## When this skill fires

The user provides a list of PSY-XXX tickets along with intent to work them in parallel worktrees. Typical phrasings:
- "Let's tackle PSY-551, PSY-552, PSY-553 in parallel worktrees"
- "Dispatch these tickets" (with a list or screenshot)
- "Work this batch in worktrees so we don't block other agents"
- A pasted screenshot of a Linear project view + "let's do these"

Also fires for the **tail-of-batch single ticket** — when a multi-ticket project sweep has wound down to one remaining ticket and the user invokes `/psy-dispatch` to continue the sweep. Common shape: a prior dispatcher's handoff message lists what shipped and what's "ready to dispatch", and the next session works the remaining ticket. Worktree isolation, background execution, and the same PR flow as the rest of the sweep are still wins; downgrading to inline work just because the count hit one creates an inconsistent tail. Do NOT, however, blindly skip the single-ticket pre-flight: still resolve ambiguity, still move to In Progress, still verify isolation, still run `/simplify`.

Do NOT use for:
- A genuine one-off ticket — no multi-ticket sweep context, no prior dispatcher handoff, the user just wants help with PSY-XXX. Do the work directly; the dispatch overhead isn't worth it when the user is actively pairing.
- A ticket whose only edits land in gitignored paths (e.g. `docs/` in this repo). Worktree edits to ignored files don't commit, don't push, don't surface in a PR, and vanish on worktree cleanup. See the anti-pattern entry below for the recovery path when this is discovered mid-flight.
- Ticket *creation* → that's `psy-ticket`.
- Generic Linear queries → that's `linear-cli`.

## Prerequisites

```bash
linear --version                  # 1.11.x verified; older may lack `issue update --state`
gh --version                      # for PR creation
git worktree list                 # confirm worktrees are usable on this checkout
test -f .linear.toml && echo ok   # team_id=PSY pinned in repo root
```

## The ironclad rules

These are the non-negotiables. They are encoded in the per-agent prompt template below; the orchestrator enforces them at dispatch time.

1. **Resolve ambiguity BEFORE dispatch.** If any ticket has an explicit design fork ("Option A or B", "pick one and document", taxonomy/threshold/UX choice not already decided), the orchestrator MUST surface those forks via `AskUserQuestion` in a single batched call before spawning any agents. See `feedback_no_speculative_implementation.md` and `feedback_plan_mode_questions_first.md`.
2. **Move tickets to In Progress on dispatch.** Before spawning agents, transition every dispatched ticket to the team's "In Progress" state. The state transition is the canonical signal to other humans/agents that work has started.
3. **Both `/simplify` AND relevant local tests run before every PR opens; failure blocks push.** No exceptions. The simplify pass lands as a SEPARATE commit if it produced edits. Local tests must be the relevant suite for what the PR touches (backend test packages, frontend unit + typecheck, plus the E2E spec if a file under `frontend/e2e/` was modified). If ANY test fails — even one the agent believes is pre-existing on main — the agent must STOP, leave the branch unpushed, and report the failure for orchestrator-level escalation. The judgment "this is pre-existing, safe to push" is NOT the agent's call to make unilaterally; pushing first and triaging via GitHub CI wastes cycles, masks the diff's true signal, and fails the engineering bar. See `feedback_simplify_before_pr.md`.
4. **One ticket = one PR.** Never bundle multiple PSY tickets into a single PR.
5. **Agents never mark Done.** Linear ticket transitions to Done happen on PR merge (which is a human call). Agents leave the ticket In Progress.
6. **Agents never merge their own PRs.** PR creation is the agent's last step; merging is the user's.
7. **Use `isolation: "worktree"` and `run_in_background: true`** on every dispatched Agent call. Running in the main worktree blocks other agents and defeats the purpose of the batch.
8. **Verify worktree isolation.** `isolation: "worktree"` is necessary but not sufficient — in the May 2026 dogfood batch, 2 of 6 agents had Edit/Write tool calls land in the main worktree's CWD instead of their isolated worktree. Each agent must verify CWD via `pwd` and `git rev-parse --show-toplevel` before editing, and must run a recovery procedure if leakage is detected (see per-agent template). The orchestrator must verify each PR's diff matches the ticket's stated scope before declaring the batch done.
9. **Verify base currency before AND after dispatch.** Worktrees branch off local main; a stale base produces stale-fallout CI failures from unrelated work that landed during dispatch. Pre-flight (before step 1): sync local main with origin/main. After step 6: re-fetch and rebase if origin/main moved during dispatch. See the stale-base anti-pattern below for the canonical May 2026 dogfix-sweep example.
10. **Manual repro before opening the PR.** Each agent must exercise the change end-to-end before pushing — local dev + screenshot for frontend, `curl` or a focused integration test for backend — and attach the artifact to the PR body's *Manual repro* section. Tests verify the contract the agent wrote (code-correct); manual repro verifies the user-facing behaviour matches the ticket (feature-correct). Per CLAUDE.md: *"Type checking and test suites verify code correctness, not feature correctness."* An empty Manual repro = the orchestrator treats the PR as unverified and escalates as a process violation.

## Workflow

### Pre-flight: sync local main + verify main CI is green

Before reading tickets, do all THREE steps. They cover three independent failure modes that compound at batch scale.

**Skill-currency check (preliminary, before Step A).** If a `psy-dispatch` SKILL.md update has been merged in this session — check via `git -C <main-repo> log --oneline -10 -- .claude/skills/psy-dispatch/SKILL.md` and look for commits whose timestamps post-date the start of your session — re-read the file via `Read .claude/skills/psy-dispatch/SKILL.md` BEFORE drafting agent prompts. Your in-memory snapshot was loaded when the skill was first invoked; if the user has merged a rule-10-style addition mid-session, prompts authored from the stale snapshot will not reflect it (e.g. agents won't run manual repro, won't `go build` before `go test`, won't use the new PR template). **Caught: May 2026 dogfix-2 (PSY-601/613/616)** — wave-2 agents shipped sound PRs but missed the just-merged manual-repro convention because the orchestrator drafted from a pre-merge snapshot.

**Step A — Confirm main repo HEAD is on `main`, then sync with origin.** Worktrees branch off the main repo's CURRENT HEAD — NOT the named `main` ref — so two failure modes compound here: (1) a non-`main` HEAD inherits unrelated side-branch commits into every dispatched PR; (2) a stale main inherits stale-fallout from work that merged in the meantime.

```bash
git -C <main-repo> branch --show-current        # must equal "main"; if not, see "Side-branch checkout" below
git -C <main-repo> fetch origin main
git -C <main-repo> log --oneline main..origin/main   # commits ahead of local?
# If main is behind:
git -C <main-repo> pull --ff-only origin main
```

**Side-branch checkout recovery.** If `branch --show-current` returns anything other than `main`, the user has a side branch checked out (commonly: a feature branch they're iterating on, a skill-update branch, an in-flight PR they're locally reviewing). Worktrees would branch off that side branch and inherit its unmerged commits into every PR. Don't dispatch yet:

1. `git -C <main-repo> status` — confirm no uncommitted changes.
2. `git -C <main-repo> log --oneline @{u}..HEAD` — confirm the side branch is in sync with its remote (no unpushed commits).
3. **If clean + synced:** surface the side-branch name to the user with a one-sentence "switching main repo to `main` for this dispatch — can switch back after if you want." Then `git switch main` and continue with the sync. Switching is non-destructive when the working tree is clean and the branch is pushed; the user can `git switch <side-branch>` back at any time.
4. **If dirty (uncommitted changes) OR has unpushed commits:** STOP and surface to user. Never auto-switch — risk of losing the user's in-flight context. Wait for explicit instruction before continuing.

Common false-flag for this check: a `pull --ff-only origin main` that fails with `"Diverging branches"` even though local main is strictly behind origin/main. The failure is misleading — it means the *currently-checked-out* branch (which isn't `main`) can't be fast-forwarded to `origin/main`, not that local main itself is divergent. Always run `branch --show-current` BEFORE diagnosing pull failures as divergence.

If `--ff-only` rejects after the side-branch check passed, the most likely cause is local main has commits not in origin/main (e.g. a stash-WIP commit). Pause and ask the user before resolving — do not blindly merge or reset.

Capture the pre-flight `origin/main` SHA so step 7 can detect movement during dispatch.

**Step B — Verify origin/main CI is currently green.** A red main propagates failure shape to every PR opened off it; agents waste cycles diagnosing failures they didn't introduce, and the orchestrator wastes cycles distinguishing batch-fault from base-fault. **The May 2026 Entity & Collections Dogfood batch (PSY-577/578/588/589)** hit exactly this: main had been red for 5+ merges on a backend tier-cap test (PSY-358 fallout) + an E2E selector mismatch (PSY-359 fallout); all 4 dispatched PRs inherited identical red CI; the dependent rebase round was wasted work that a five-second pre-flight would have prevented.

```bash
gh run list --branch main --limit 4 --json conclusion,status,headSha,displayTitle
```

Read the most recent run with `status: "completed"` (skip in-progress runs from a recent merge — they're not yet decisive). Decision tree:

- `conclusion: "success"` → main is green; proceed to step 1.
- `conclusion: "failure"` (or `"cancelled"` / `"timed_out"`) → main is red. **STOP and surface to the user** with the failing run URL + diagnosed cause if quickly identifiable (look for repeated failures across recent runs — that's the steady-state failure shape, not a single flake). Choose one of:
  - **Fix main first via an inline CI-restoration ticket.** Recommended. Canonical example: **PSY-611 (May 2026)** — single PR off red main, two test fixes (backend `CreateTestUser` → `CreateAdminUser` for tier-cap, E2E selector update for popover rebuild), ~30 min from filing → merged. The dispatched batch then rebases onto green main and ships clean. Trades a small upfront delay for zero rebase rounds and clean per-PR CI signal.
  - **Accept red base.** Dispatch anyway with explicit per-agent context: *"origin/main CI is currently failing on `<failure name>`; ignore that specific failure, focus on whether YOUR diff introduces NEW failures."* High judgment cost on the agent; not recommended unless the base-fix is genuinely out of scope and the user explicitly opts in.
  - **Hold the dispatch entirely.** Wait for someone else to fix main; surface back when CI is green.
- All recent completed runs are in-progress or pending → wait briefly (`gh run watch <id>` on the latest), or surface to user with the in-flight context.

Do NOT silently dispatch on red main and hope CI gets fixed before merge — wasted CI cycles + muddled per-PR signal are real costs that compound across batch size.

**Step C — Audit other in-flight work in the repo.** The orchestrator's job isn't just file-level conflict avoidance for THIS batch but also conceptual-scope avoidance: another agent (a separate Claude session, or a human) may be migrating a primitive your tickets consume, or a recently-merged PR may have invalidated a ticket's premise.

```bash
git -C <main-repo> worktree list                                                    # other agents' active worktrees → their in-flight branches
gh pr list --state open --limit 10 --json number,title,headRefName,updatedAt        # open PRs (in-flight or queued for merge)
linear issue list --team PSY --state "In Progress" --all-assignees                  # tickets currently being worked
```

Cross-reference the tickets you're about to dispatch against active worktrees + open PRs to identify:
- **File overlap**: another agent is editing files your ticket would touch → defer or coordinate.
- **Conceptual overlap**: another agent's ticket modifies a primitive (resolver, drawer component, route group) your ticket consumes → wait for theirs to merge first so your ticket consumes the new shape.
- **Recent invalidation**: a recently-merged PR (last few hours) may have already addressed your ticket's root cause → re-read the ticket against current code before dispatching, or close it as auto-resolved.

**Caught: May 2026 dogfix-1 (PSY-604/615)** — surfacing another agent's PSY-608/609/610/612 scope made it possible to pick non-overlapping tickets from the start. Without this check, the two batches would have produced colliding PRs in the comment + collection + user-resolver areas.

**Per-ticket branch + worktree + PR cross-check** (in addition to the broad audit above). For each ticket in this batch, also cross-check whether a branch already exists locally or remotely. The broad worktree/PR list above catches active scope overlap; this narrower per-ticket check catches the orphaned-worktree / parallel-session-mid-flight case:

```bash
git -C <main-repo> branch -a | grep -iE "PSY-{N}( |/|$)"
git -C <main-repo> worktree list | grep -iE "PSY-{N}( |/|$)"
gh pr list --search "PSY-{N}" --state all --json number,state,title,url
```

If any check turns up a match — a branch exists, a worktree holds it, or any PR (open / merged / closed) is already in place for that ticket — the ticket is NOT a fresh dispatch. See **"Take-over flow when prior partial work exists"** below for the disposition decision tree. Do NOT dispatch a fresh agent on a ticket whose branch is already held by another worktree — `git checkout -b` will fail at the agent's first step, and force-deleting the branch would silently destroy the parallel session's work.

### 1. Read every ticket in parallel

```bash
linear issue view PSY-551 --json
linear issue view PSY-552 --json
# ... one bash call per ticket, all in a single assistant message so they run concurrently
```

Scan each description for:
- Explicit "decision required" / "Option A vs B" / "pick one" language
- Acceptance criteria
- Pointers to related work (PSY-XXX references, file paths, prior-art examples)
- Scope blast radius (cross-cutting? local? backend+frontend?)

### Take-over flow when prior partial work exists

When the per-ticket branch + worktree + PR cross-check (Step C) turns up a match for a ticket in this batch, you have a parallel-session race or a paused-mid-flight ticket. Three dispositions, ranked by value:

1. **Take over from the orchestrator (preferred when work is non-trivial).** Inspect the worktree state — `git -C <worktree-path> log main..HEAD --oneline` for committed work, `git -C <worktree-path> status` for uncommitted edits, `gh pr list --search "PSY-{N}"` for any opened PR. If the work is substantive (matches the ticket's AC partially or completely), the orchestrator handles the rest directly — verify the existing commits against AC, run typecheck + tests + manual repro per the per-agent template's steps 4–5, run `/simplify` if not already done, push and open the PR (or `gh pr edit --body` if a PR is already open against an outdated convention). DO NOT dispatch a fresh agent on top — they'd race with whoever was working in that worktree, the branch checkout would fail, and force-deleting would destroy the parallel session's work.

2. **Discard and dispatch fresh (when prior work is sparse or wrong-direction).** If the worktree contains only a few uncommitted edits in the wrong direction OR untracked files that miss the ticket scope, **ask the user before destroying the work**. Use `git restore` for tracked files, surface untracked files with their paths so the user can decide. Then `git worktree remove <path> --force` to free the branch, `git branch -D <branch-name>`, and dispatch a fresh agent.

3. **Skip this ticket in the wave.** If you can't tell whether the parallel session is still actively working — recent commits within the last few minutes, an active claude process holding the worktree — leave the ticket in its current state and surface to user. Don't race.

**Race-condition mitigation:** if a parallel session may still be active, defer the take-over to a separate orchestrator turn. Pushing or editing the worktree mid-flight to another session would corrupt their state.

**Canonical example: PSY-613 (May 2026).** A parallel claude session had partial work in `agent-ab5cb884f857468a0`. When the orchestrator's pre-flight tried to delete what looked like an empty branch, the delete failed because the worktree held it. Inspecting the worktree showed: 2 commits (initial implementation + simplify pass), clean working tree, no PR yet. The orchestrator took over (verified, pushed, PR'd, edited the PR body to follow the new convention). Result: clean PR #563 without a competing agent, no wasted CI cycle, and the parallel session's work was preserved.

### 2. Surface ambiguity (mandatory)

For every ticket that has a design fork, build a question for `AskUserQuestion`. Batch all questions into a single call (max 4 per round; if >4 the batch is too big and should be split into two dispatches).

For each question:
- Put the recommended option first, suffixed with "(Recommended)" if the ticket itself recommends one.
- Provide a `description` for each option that summarizes the trade-off in one sentence.
- Don't ask about things the agent can determine from code (file paths, function names, type signatures).

If no ambiguity exists across the batch, skip this step and proceed.

### 3. Move tickets to In Progress

Issue one `linear issue update PSY-XXX --state "In Progress"` per ticket, in parallel:

```bash
linear issue update PSY-551 --state "In Progress"
linear issue update PSY-552 --state "In Progress"
# ... etc, all in a single assistant message
```

The PSY team's In-Progress state is literally named `"In Progress"` (case-sensitive, with the space). Verified against the live workspace; do not substitute `"Started"` or `"In progress"`.

### 4. Dispatch agents in parallel (single message, multiple Agent calls)

Spawn one Agent per ticket with this exact configuration:

```
Agent({
  description: "PSY-XXX <short>",
  subagent_type: "general-purpose",
  isolation: "worktree",
  run_in_background: true,
  prompt: <the per-agent prompt template, filled in>
})
```

Send all Agent calls in a SINGLE assistant message so the harness runs them concurrently. The `isolation: "worktree"` flag creates an isolated worktree per agent and auto-cleans worktrees that produce no changes. `run_in_background: true` means you'll be notified on completion — do NOT poll.

### 5. Wait for notifications

The harness notifies you as each agent completes. Do not poll, sleep, or check on progress. Use the time for unrelated work or end the turn.

### 6. Surface results

When all agents have returned (or as each returns):
- Present each ticket's branch, PR URL, and one-line summary.
- Flag any agent that reported a STOP / blocker / open question — that takes precedence over reporting the others.
- Do NOT include diffs in the summary. PRs carry the diff.
- **Verify isolation post-hoc.** Run `git status` from the main repo and `gh pr view <PR> --json files` for each PR. Each PR's file list should match the ticket's stated scope; the main repo should have no uncommitted changes from any agent (only the pre-existing untracked files from session start). If you find leakage, the agent's recovery procedure should have handled it — but verify rather than trust.
- **Verify PR body convention compliance.** For each PR opened, confirm the body contains the current convention's required sections:
   ```bash
   gh pr view <PR> --json body --jq .body | grep -cE "^## (Manual repro|Simplify)"
   # Expected: 2 — one ## Manual repro header + one ## Simplify header.
   ```
   If sections are missing — typically because a parallel session opened the PR off a stale skill snapshot, OR an orchestrator-takeover inherited a PR opened before a convention update merged — edit the body via `gh pr edit <PR> --body "<new content>"` to bring it up to spec. Reuse the agent's return-message details (Manual repro and Simplify outcomes) verbatim where possible; the convention exists so reviewers can audit verification from the PR alone, not from the agent's chat trace. **Caught: May 2026 dogfix-2 (PR #563)** — parallel session opened PSY-613's PR before PR #559 (the convention-adding skill update) merged; orchestrator took over and edited the body to add the missing `## Manual repro` + `## Simplify` sections. Without this check, drift accumulates: each merged off-spec PR sets a precedent that the next reviewer accepts.

### 7. Stale-base recovery + apply orchestrator-pending memory entries

After step 6, re-fetch origin/main. If it moved during dispatch, the PR bases are now stale and CI may fail with stale-fallout from work that merged mid-dispatch.

```bash
git -C <main-repo> fetch origin main
git -C <main-repo> log --oneline <pre-flight-origin-main-SHA>..origin/main   # moved?
gh pr checks <PR>   # if origin/main moved, check CI on each PR
```

**Symptom of stale-base:** identical CI failure shape across multiple PRs that touch different files (e.g. the same Backend test name failing on every PR in the batch, while each PR's frontend unit tests pass). The diffs themselves are clean. If the failing tests reference work that merged into origin/main during dispatch (look at the test file's git blame), it is almost certainly stale-base, not the diff.

**Recovery** — rebase each affected worktree onto origin/main and force-push. Parallelisable across worktrees (each operates on its own branch):

```bash
git -C <worktree> rebase origin/main && \
git -C <worktree> push --force-with-lease origin <branch>
```

Always `--force-with-lease`, never `--force` — bails out if the remote moved (someone pushed during the rebase) instead of overwriting their work. If a rebase produces conflicts, stop and surface the conflict to the user; don't auto-resolve.

**Apply orchestrator-pending memory entries.** For each agent that returned a *Proposed memory entries* block in its report (because no in-repo `CLAUDE.md` existed), work through this checklist after PRs are pushed and CI is clean:

1. **Re-read user-level `MEMORY.md` BEFORE editing.** The user may be doing parallel housekeeping during the batch — moving long entries into topic files (e.g. `pattern_*.md` pointers in the index), updating in-place caveats as related work merges, or normalising entry structure. The cached file content from your earlier read may be stale by the time you go to apply entries; `Edit` will fail with `"String to replace not found in file"` and you'll waste a turn on diagnosis. **Caught: May 2026 dogfix-2** — user updated the PSY-353 entry in-place (added PSY-612 reference, dropped the in-flight caveat) while orchestrator was working from a stale read; the agent's proposed text targeted the OLD entry shape and the Edit failed. Re-read first, then resolve any drift between the agent's proposal and the current entry shape, then edit.
2. Read the proposed entry text from the agent's return report.
3. Locate the target section header in user-level `MEMORY.md` (the agent should have named it; if not, find by topical fit).
4. Append, OR replace if the new entry resolves an existing caveat (e.g. PSY-612 dropped the "canonical chain is NOT project-wide" caveat from the PSY-353 entry). Keep any one-line index pointer in `MEMORY.md`'s top-level index under ~150 chars per the existing memory rules.
5. After all entries are applied, verify total `MEMORY.md` size is still under the index-loading limit (`MEMORY.md` shows a warning at the top when overrun). If close, move the longest entries into per-topic files and leave only the index pointer in `MEMORY.md`.

The orchestrator owns the user-level memory file; agents do not edit it from inside their worktrees. Skipping this checklist means the next dispatch operates on stale memory.

## Per-agent prompt template

Each agent's prompt must be SELF-CONTAINED (the agent has none of this conversation's context). Fill in the placeholders for each ticket. Keep the conventions block identical across agents.

```markdown
Fix PSY-{N}: {ticket title}.

# Decision (already made — do NOT pick differently)
{If a design decision was resolved in step 2, paste it here verbatim. Otherwise omit this section.}

# Repo context
- Repo root: psychic-homily-web. Dual codebase: `backend/` (Go, Huma + GORM) and `frontend/` (Next.js).
- Branch from `main`. Branch name: `PSY-{N}/{kebab-short-description}`.
- Commit format: imperative, subject `PSY-{N}: <summary>`, HEREDOC body, `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- PR title: `PSY-{N}: <summary>` (under 70 chars). PR body must include `Closes PSY-{N}`.
- **Memory edits**: if acceptance criteria call for a "CLAUDE.md note" or "project memory update", target the in-repo `CLAUDE.md` (so the edit lands in the PR alongside the code change). DO NOT edit the user-level `MEMORY.md` at `~/.claude/projects/.../memory/MEMORY.md` from inside your worktree — that file is outside the repo and outside the PR; edits to it bypass review. If no in-repo `CLAUDE.md` exists, skip the file edit and return the proposed entry verbatim in your report under "Proposed memory entries"; the orchestrator applies it post-batch with full visibility.

# Problem
{paste from ticket description}

# Acceptance criteria
{paste from ticket}

# Pointers
{2–6 bullets on where to look — prior-art files, related shipped tickets, framework primitives. Helps the agent skip the discovery phase. If you don't know the file paths, say so and let the agent grep.}

**If a research/audit doc is cited in this Pointers section** (`docs/research/*.md`, audit deliverables): treat its counts/sites/claims as point-in-time, NOT authoritative. Re-verify against current code in step 2 before relying on them. Audit docs drift fast; **PSY-610 (May 2026)** found an audit claimed 10/10 silent surfaces when only 5/11 were actually silent post-prior-work; **PSY-612 (May 2026)** found a 6th call site the user-attribution audit missed. Trust current code over the doc — per `feedback_no_speculative_implementation.md` and CLAUDE.md "distinguish 'the doc says X' from 'X is currently true'".

# Work plan
1. **Verify isolation FIRST.** Run `git rev-parse --show-toplevel`. It must resolve under `.claude/worktrees/`, not the main repo root.
2. Explore: {what to read first}
3. Implement the fix.
4. **Run all relevant local tests. Failure blocks push.** This is non-negotiable. Run, in order of how directly they exercise your diff:
   - **Backend changes (build first, then test):** `cd backend && go build ./...` BEFORE `go test`. Build catches whole-graph compile errors (missed call sites after a refactor, broken imports across packages); tests catch behaviour. **PSY-612 (May 2026)** caught a sixth user-resolver call site (`services/admin/entity_report.go`, sharing the package-private `displayName` helper with `pending_edit.go`) at `go build` time that the audit doc had missed — without the build pre-step, this would have been runtime-discovered post-merge. Then `go test ./<package(s) you touched>/...` — target the package(s) you edited plus any package whose tests directly exercise the changed surface. If the diff is large, run `go test ./...`.
   - **Frontend type safety:** `cd frontend && bun run typecheck`.
   - **Frontend unit tests:** `cd frontend && bun run test:run <relevant scope>` (e.g. `bun run test:run features/comments`). The actual scoped runner is `test:run`; `test:unit` does not exist as a script, and `--`-prefixed argument-passing is not how the runner accepts a path filter — confirm via `package.json` `scripts` if uncertain.
   - **E2E:** if you modified any file under `frontend/e2e/`, run that spec — `cd frontend && bun run test:e2e -- <path-to-spec>`. The E2E global-setup hard-requires port 8080 to be free; if the user's dev backend occupies 8080, STOP and report back so the orchestrator can ask the user to free it. Do NOT skip the E2E run silently.
   - **Docs-only PRs (no code changes):** if your diff touches ONLY non-functional docs — markdown files, `.claude/skills/*/SKILL.md`, README updates, comment-only changes — there is no code path to exercise and no functional tests to run. Note `"docs-only, no tests applicable"` in your "Local tests run" line and proceed. **Exception:** if the same diff also touches a config/build file (`package.json`, `go.mod`, `tsconfig.json`, `playwright.config.ts`, `Makefile`, CI workflow YAML), run the corresponding typecheck or build to confirm nothing broke at that boundary.
   - **STOP if any test fails.** Do not try to debug whether the failure is "pre-existing" or whether your diff caused it — that's the orchestrator's call, and the orchestrator will escalate to the user. Report back with: failing test name, error excerpt, the exact command you ran, and your one-sentence hypothesis. Do NOT proceed to commit/push. The judgment "this is pre-existing on main, safe to push" is NOT yours to make. Pushing untested or known-failing code is the single worst pattern this skill exists to prevent.
5. **Manual repro the change end-to-end.** Tests verify the contract the agent wrote; manual repro verifies the user-facing behaviour matches the ticket. Skipping this because "tests passed" fails the engineering bar — per CLAUDE.md: *"Type checking and test suites verify code correctness, not feature correctness."*
   - **Frontend changes:** start the dev server on a FREE PORT (e.g. `cd frontend && PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()") && bun run dev --port $PORT`) — sharing port 3000 with the orchestrator or other parallel agents will fail. Connect to the existing dev backend at the standard port (read paths and most write paths share fine in this repo; if the change exercises rate-limited or singleton state, flag it and serialize across the batch). Use `chrome-devtools` MCP or `agent-browser` to navigate to the affected screen, exercise the canonical failing path the ticket described, and capture a screenshot of the new behaviour into `dogfood-output/PSY-{N}/screenshots/<short-name>.png`. STOP if the canonical failure mode does NOT now surface in the UI — the fix is incomplete; iterate from step 3 before proceeding.
   - **Backend changes: integration tests are the canonical manual repro.** Write or extend a focused integration test that drives the change end-to-end through the real stack — exact-message assertions, response-shape assertions, all AC cases covered. **PSY-592 (May 2026)** is the canonical example: three tests (`_EmptyPermission`, `_InvalidEnum`, `_AcceptsAllValidEnumValues`), each asserting the exact response body. The test name + assertion outcome is what goes in the PR body's *Manual repro* section. Use `curl` against a backend you started on a free port ONLY when the test harness genuinely can't reach the path (rare — most paths have a test entrypoint). Capture the request + response (or test output) verbatim. STOP if the response shape diverges from the ticket's expectation.
   - **Docs-only PRs (no code path):** no manual repro applicable. Note `"docs-only, no manual repro applicable"` in your report and PR body.
   - **Render-only refactor carveout.** Pure refactors that don't change behaviour (extracting a primitive across N existing call sites, renaming a prop, consolidating duplicate render logic) verify the user-facing surface via unit tests asserting on rendered DOM output. If the local dev environment can't run end-to-end (backend unavailable on standard port, port conflict, DB seed missing), the agent MAY proceed with an honest-disclosure Manual repro section: *"Unit tests at `<file>` (N lines, M cases) cover the rendered output for all affected surfaces. Local navigation-level smoke skipped because <reason>. Recommended pre-merge: spot-check on Vercel preview or local-with-backend."* This is **NOT a free pass** to skip manual repro — it's specifically for refactors where unit-test DOM assertions cover what manual repro would verify, AND the local environment genuinely can't run. Surface the limitation explicitly per CLAUDE.md "if you can't test the UI, say so explicitly rather than claiming success." Most often invoked during an orchestrator-takeover (Step 1's take-over flow) of a render-only refactor where the dev backend isn't running. **Canonical example: PSY-613 (May 2026)** — orchestrator-takeover of a `<UserAttribution />` primitive extraction (10 inline implementations replaced); 3080 unit tests + 137-line `UserAttribution.test.tsx` covered the rendered output; backend not running locally; PR body explicitly disclosed the gap and recommended a pre-merge spot-check.
6. **Pre-commit isolation check.** Run `git status` from your worktree. Then run `git -C <main-repo-path> status` (the main repo absolute path). If the main repo shows YOUR file changes uncommitted, the harness CWD didn't propagate — recovery procedure:
   - Copy your edits from the main repo into your worktree (`cp` with absolute paths).
   - In the main repo, `git restore <leaked-paths>` to revert (use `git restore`, not `git checkout .` or `git clean` — both can wipe unrelated untracked files).
   - Verify `git status` in main shows only the pre-existing untracked files from session start.
   - Continue from your worktree.
7. Commit the implementation.
8. Run `/simplify` (Skill tool, skill: "simplify"). If it edited files, commit them as a SEPARATE commit `PSY-{N}: simplify pass`. **Re-run the relevant local tests from step 4** if simplify changed anything substantive. Re-run the manual repro from step 5 only if simplify edited a file you exercised in step 5.
9. Push branch with `-u origin <branch>`.
10. Open PR with `gh pr create`. Body template:
    ```
    ## Summary
    - <bullet 1>
    - <bullet 2>

    ## Test plan
    - [x] <command you ran locally> — passed
    - [x] <command you ran locally> — passed

    ## Manual repro
    <Frontend: link to screenshot at `dogfood-output/PSY-{N}/screenshots/<name>.png` + one-sentence description of what the screenshot shows. Backend: exact `curl` command + response body verbatim, OR test name + relevant assertion output. State what you exercised — the canonical failing path from the ticket — and what you saw. "docs-only, no manual repro applicable" is the only valid placeholder.>

    ## Simplify
    <one-line outcome: "no changes" OR "edited N files, -M net lines, <one-phrase summary>". Post-simplify retest commands belong in the Test plan above with [x].>

    Closes PSY-{N}
    ```
    The Test plan section must list the actual commands you ran in step 4, with `[x]` checkboxes (not unchecked) — they're statements of "I verified this", not aspirations. The Manual repro section is the artifact from step 5; without it the PR is unverified and the orchestrator escalates as a process violation. The Simplify section makes the simplify outcome auditable from the PR alone, not just the agent's return-message.

# Reporting back
Short report (under 300 words):
- Branch + worktree path; PR URL (or "not pushed — see Local tests run below" if you stopped on test failure)
- Files changed (count + brief category breakdown)
- Behaviour change (one or two sentences)
- **Local tests run (REQUIRED):** list every command you ran from step 4 and its outcome ("ok", "FAIL: <test name> — <one-line excerpt>"). If you skipped a class because it wasn't relevant to the diff, say so explicitly with one-sentence justification. An empty/missing field = orchestrator treats the PR as untested and escalates as a process violation.
- **Manual repro (REQUIRED):** what you exercised in step 5 and what you saw — mirrors the PR body's *Manual repro* section. Frontend: screenshot path + observed behaviour. Backend: command + observed output, or integration-test name + assertion outcome. Empty/missing = orchestrator treats the PR as unverified and escalates as a process violation.
- `/simplify` diff (or "no changes"). If simplify changed code, list the post-simplify re-run of the test commands from step 4. Manual repro re-run only if simplify edited a file you exercised in step 5.
- Isolation check: clean, or tripped + recovered
- **Proposed memory entries** (only if relevant): if your acceptance criteria called for a memory/CLAUDE.md note and no in-repo `CLAUDE.md` exists to land it in-PR, paste the proposed entry verbatim and identify the target section header in user-level `MEMORY.md` (e.g. "Key Non-Obvious Patterns"). Orchestrator applies post-batch.
- Scope-adjacent observations: out-of-scope patterns / refactors / warnings noticed. Do NOT expand PR scope to address them.
- Blockers / open questions

No full diff. Don't mark Done in Linear (happens on merge). Don't push to main. If you discover an unsurfaced design ambiguity during exploration OR any local test fails, STOP and report back instead of guessing or pushing.
```

## Anti-patterns

These supplement the ironclad rules with tactical guidance from observed batch failures. Rule restatements have been omitted — see "The ironclad rules" above.

- **Skipping `/simplify` for "small" tickets.** The discipline is the point. Most small tickets produce no simplify diff anyway; running it costs nothing.
- **Pushing past failing local tests by labeling them "pre-existing on main".** **PSY-588 (May 2026)** ran `go test ./...`, observed `TestCollectionHandlerIntegration/TestGetUserCollectionsContaining_OnlyMatchingCollections` failing in the `community` handlers package, judged it "unrelated to PSY-588 — reproduced on stashed main", and pushed PR #547 anyway. CI failed on the same test the agent had already seen locally — wasted CI cycle, PR looked broken to a casual reviewer despite the diff being clean, and the engineering-bar signal it sent ("agents push without testing their changes") triggered the user-feedback that produced rule 3 above. The judgment "this is pre-existing, safe to push" is NOT the agent's call to make unilaterally — STOP, escalate to the orchestrator, and let the user decide between (a) fixing the flake first (canonical recovery: a CI-restoration ticket like PSY-611 ran inline before the dependent batch lands), (b) skipping the test, (c) accepting the noise. Even when the agent's diagnosis is correct, the wasted cycle and the bar-setting cost is real. Encoded in rule 3 + step 4 of the work plan; this entry exists to keep the incident named so the cost stays visible.
- **Skipping the E2E run because the user's dev backend occupies port 8080.** E2E global-setup hard-checks port 8080 and refuses to start the test backend if anything is listening. The right move when the agent (or orchestrator) hits this is to STOP and ask the user to free port 8080 — not to skip E2E and push a frontend `e2e/` change unverified. Caught on PSY-611 (May 2026) where the user had a dev backend running locally; freeing it took ~10 seconds and unblocked the verification.
- **Opening a side-PR off stale main while a base-fix PR is still in flight.** If you open a separate-purpose PR (a docs-only skill update, an unrelated tooling tweak, etc.) while a CI-restoration / base-fix PR is still open and unmerged, your side-PR's branch is created from main BEFORE the fix lands and inherits the broken base. **PR #551 (May 2026)** hit this: the skill update was opened off main while #550 (PSY-611 CI restoration) was still in review; #551 inherited #550's red CI shape until #550 merged and #551 was rebased + force-pushed. Wasted one extra CI cycle. Either wait for the base-fix to merge before opening the side-PR, or commit upfront to rebasing it afterward and budget for the extra cycle.
- **Trusting `isolation: "worktree"` blindly.** In the May 2026 dogfood batch (PSY-551 through PSY-556), 2 of 6 agents had Edit/Write tool calls land in the main worktree's CWD despite the isolation flag. The agents that detected and recovered (copy-edits-to-worktree → `git restore` leaked paths in main → resume) shipped clean PRs; without the recovery they would have committed the wrong files to the wrong branch. Always verify isolation up front and pre-commit, and run the orchestrator-level diff check at step 6.
- **Using `git checkout .` or `git clean -fd` to "reset" main during recovery.** Both can wipe unrelated untracked files in the main worktree (e.g. another in-flight WIP, or session-scope draft files like a new skill). Use `git restore <specific paths>` only — target the leaked paths explicitly.
- **Dispatching a ticket whose targets are all gitignored.** A worktree creates an isolated branch, but edits to gitignored paths live only in the worktree's filesystem — they don't commit, don't push, don't reach a PR, and disappear when the worktree is cleaned up. **PSY-427 (May 2026)** hit this: the target was `docs/runbooks/agent-workflow.md` + `docs/INDEX.md`, and `docs/` is in `.gitignore`. Pre-flight check before step 4: run `git check-ignore -v` against each target file the ticket calls out (or run it against the entire `docs/` tree if the ticket is a docs-only update). If everything is ignored, abort the dispatch and do the work inline on main — the user reviews the diff in-conversation, accepts, and the ticket transitions Done directly. There is no merge event to gate on.
- **Dispatching from a stale local main (whole-batch CI failure).** Worktrees branch off local main; if it's behind origin/main at dispatch time, every PR inherits the same stale base. **The May 2026 dogfix sweep (PSY-558/559/560/561/562)** hit this: local main was 8 commits behind origin/main; two of those commits (PSY-357 + PSY-359) added test files exercising new collection paths; ALL 5 PRs failed the same Backend + E2E suites despite each PR's diff being clean and unrelated to collections. Frontend unit tests passed on every PR — the only suite actually exercising the diff. The signature is **identical CI failure shape across PRs that touch different files**. Pre-flight (sync local main before step 1) catches the stale-at-dispatch case; step 7 catches the moved-during-dispatch case. Recovery is a parallel `git rebase origin/main && git push --force-with-lease origin <branch>` per worktree (per step 7).
- **Agents writing project-pattern docs to user-level MEMORY.md from inside their worktree.** When the per-agent prompt says "add a CLAUDE.md note" but no project-level `CLAUDE.md` exists in the repo, agents fall through to the user-level memory file at `~/.claude/projects/<project>/memory/MEMORY.md` — which sits OUTSIDE the worktree, OUTSIDE the repo, and OUTSIDE the PR. Same shape as the gitignored-target anti-pattern: edits that don't reach review. **PSY-558 + PSY-559 (May 2026)** both did this; content was correct and ended up in the right file, but it bypassed PR review and bypassed orchestrator visibility. Fix: the per-agent template's *Repo context* + *Reporting back* sections instruct agents to edit in-repo `CLAUDE.md` if present (lands in the PR), otherwise return the proposed entry in their report under *Proposed memory entries* — the orchestrator applies user-level `MEMORY.md` updates in step 7 with full visibility.
- **Dispatching while the main repo HEAD is on a side branch.** The harness's `isolation: "worktree"` flag creates each worktree off the main repo's CURRENT HEAD, not the named `main` ref. If the user has a feature/skill-update branch checked out at dispatch time, every dispatched PR would inherit that branch's unmerged commits — including the commit that branch was iterating on. **May 2026 dogfix-2 dispatch (PSY-601/613/616)** caught this: orchestrator's `pull --ff-only origin main` failed with `"Diverging branches"` even though local main was strictly behind origin/main; root cause was that the user had `dispatch-skill-level-a-and-fixes` checked out (their in-flight skill iteration). Without the Step-A `branch --show-current` guard, the dispatch would have produced 3 PRs each carrying a stray skill-update commit. Fix encoded in Step-A: check HEAD before sync, switch to `main` (with announcement) if the side branch is clean and synced, STOP and ask if it has uncommitted or unpushed work.
- **Calling `gh pr view --json merged` instead of `--json mergedAt`.** The field is `mergedAt` (ISO timestamp string when merged, `null` when not). `merged` doesn't exist; the call returns `Unknown JSON field: "merged"` and lists valid fields. Use `--json state,mergedAt` to read both at once. Also useful for the same check: `--json state,statusCheckRollup` for the merged-and-CI-was-green compound check. Caught: May 2026 dogfix-2.
- **Running `git pull` / `git status` from the orchestrator without `-C <main-repo>` after agents return.** The harness's CWD propagation can leave the orchestrator's shell inside a worktree that just completed; subsequent unscoped `git` calls then run inside the worktree, not the main repo. Symptom: `git pull --ff-only origin main` failing in unexpected ways, or `git status` reporting on the wrong branch. Distinct from the side-branch HEAD anti-pattern: that one is the user's choice, this one is harness CWD propagation. **Always use `git -C /Users/mtrifilo/dev/psychic-homily-web` (or `git -C <main-repo>`) explicitly** for main-repo operations after dispatch returns. If unsure, `pwd` first to verify CWD before running unscoped `git`. Caught: May 2026 dogfix-2 — orchestrator's `pull --ff-only` from the main repo path was actually executing inside a returned-agent worktree because Bash CWD had shifted there.
- **Drafting agent prompts from a stale skill snapshot when a SKILL.md update has merged mid-session.** Encoded in the new skill-currency check at the top of pre-flight; this entry exists to keep the cost named so the next orchestrator doesn't repeat it. **Caught: May 2026 dogfix-2** — wave-1 orchestrator's snapshot of psy-dispatch SKILL.md was loaded BEFORE the user merged a rule-10 update adding the manual-repro requirement. Wave-2 prompts (PSY-601/613/616) were authored from that pre-merge snapshot; agents shipped sound PRs but skipped the new manual-repro / `go build`-first / new-PR-template conventions. Re-reading the file before dispatch would have caught it.

## Related skills and memories

- **`psy-ticket`** — ticket *creation* (this skill is for ticket *execution*).
- **`linear-cli`** — generic Linear CLI surface; drop down to it if `linear issue update --state` lacks a flag you need.
- **`simplify`** — invoked by every dispatched agent before opening its PR.
- `feedback_simplify_before_pr.md` — `/simplify` AND relevant local tests run before every PR (single-ticket or batched); failure blocks push, escalate to orchestrator instead of pushing past it.
- `feedback_no_speculative_implementation.md` — when a ticket is ambiguous about WHAT to build, STOP and ask.
- `feedback_plan_mode_questions_first.md` — surface forks via `AskUserQuestion` before exiting plan mode / dispatching.
- `feedback_code_complete.md` — manage complexity, plan before coding, decompose big changes.
- `feedback_verify_before_push.md` — verify the fix actually fixes the thing before pushing.
