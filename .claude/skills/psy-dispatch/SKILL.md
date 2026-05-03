---
name: psy-dispatch
description: Dispatch a batch of Psychic Homily Linear tickets in parallel git worktrees. Use when the user provides a list of PSY-XXX tickets and asks to "tackle in parallel", "dispatch in worktrees", "batch these tickets", or otherwise work multiple tickets concurrently without blocking each other. One agent per ticket, each in its own isolated worktree, each runs `/simplify` before opening its PR. Resolves ambiguity via AskUserQuestion BEFORE dispatch.
argument-hint: "[PSY-XXX, PSY-YYY, ... or screenshot of Linear list]"
---

# psy-dispatch: parallel-worktree ticket dispatch

Encodes the workflow for taking a batch of PSY tickets and dispatching one parallel-worktree agent per ticket. Built on top of `psy-ticket` (which owns ticket *creation*); this skill owns ticket *execution*.

## When this skill fires

The user provides a list of two or more PSY-XXX tickets along with intent to work them in parallel. Typical phrasings:
- "Let's tackle PSY-551, PSY-552, PSY-553 in parallel worktrees"
- "Dispatch these tickets" (with a list or screenshot)
- "Work this batch in worktrees so we don't block other agents"
- A pasted screenshot of a Linear project view + "let's do these"

Do NOT use for:
- A single ticket → just do the work directly. One ticket doesn't need a parallel-dispatch workflow.
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
3. **`/simplify` runs before every PR opens.** No exceptions. See `feedback_simplify_before_pr.md`. The simplify pass lands as a SEPARATE commit so the diff is independently reviewable.
4. **One ticket = one PR.** Never bundle multiple PSY tickets into a single PR.
5. **Agents never mark Done.** Linear ticket transitions to Done happen on PR merge (which is a human call). Agents leave the ticket In Progress.
6. **Agents never merge their own PRs.** PR creation is the agent's last step; merging is the user's.
7. **Use `isolation: "worktree"` and `run_in_background: true`** on every dispatched Agent call. Running in the main worktree blocks other agents and defeats the purpose of the batch.
8. **Verify worktree isolation.** `isolation: "worktree"` is necessary but not sufficient — in the May 2026 dogfood batch, 2 of 6 agents had Edit/Write tool calls land in the main worktree's CWD instead of their isolated worktree. Each agent must verify CWD via `pwd` and `git rev-parse --show-toplevel` before editing, and must run a recovery procedure if leakage is detected (see per-agent template). The orchestrator must verify each PR's diff matches the ticket's stated scope before declaring the batch done.

## Workflow

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

# Problem
{paste from ticket description}

# Acceptance criteria
{paste from ticket}

# Pointers
{2–6 bullets on where to look — prior-art files, related shipped tickets, framework primitives. Helps the agent skip the discovery phase. If you don't know the file paths, say so and let the agent grep.}

# Work plan
1. **Verify isolation FIRST.** Run `git rev-parse --show-toplevel`. It must resolve under `.claude/worktrees/`, not the main repo root.
2. Explore: {what to read first}
3. Implement the fix.
4. Run typecheck / relevant tests.
5. **Pre-commit isolation check.** Run `git status` from your worktree. Then run `git -C <main-repo-path> status` (the main repo absolute path). If the main repo shows YOUR file changes uncommitted, the harness CWD didn't propagate — recovery procedure:
   - Copy your edits from the main repo into your worktree (`cp` with absolute paths).
   - In the main repo, `git restore <leaked-paths>` to revert (use `git restore`, not `git checkout .` or `git clean` — both can wipe unrelated untracked files).
   - Verify `git status` in main shows only the pre-existing untracked files from session start.
   - Continue from your worktree.
6. Commit the implementation.
7. Run `/simplify` (Skill tool, skill: "simplify"). If it edited files, commit them as a SEPARATE commit `PSY-{N}: simplify pass`.
8. Push branch with `-u origin <branch>`.
9. Open PR with `gh pr create`. Body template:
   ```
   ## Summary
   - <bullet 1>
   - <bullet 2>

   ## Test plan
   - [ ] <concrete check>
   - [ ] <concrete check>

   Closes PSY-{N}
   ```

# Reporting back
Short report (under 250 words):
- Branch + worktree path; PR URL
- Files changed (count + brief category breakdown)
- Behaviour change (one or two sentences)
- `/simplify` diff (or "no changes")
- Isolation check: clean, or tripped + recovered
- Scope-adjacent observations: out-of-scope patterns / refactors / warnings noticed. Do NOT expand PR scope to address them.
- Blockers / open questions

No full diff. Don't mark Done in Linear (happens on merge). Don't push to main. If you discover an unsurfaced design ambiguity during exploration, STOP and report back instead of guessing.
```

## Anti-patterns

These supplement the ironclad rules with tactical guidance from observed batch failures. Rule restatements have been omitted — see "The ironclad rules" above.

- **Skipping `/simplify` for "small" tickets.** The discipline is the point. Most small tickets produce no simplify diff anyway; running it costs nothing.
- **Trusting `isolation: "worktree"` blindly.** In the May 2026 dogfood batch (PSY-551 through PSY-556), 2 of 6 agents had Edit/Write tool calls land in the main worktree's CWD despite the isolation flag. The agents that detected and recovered (copy-edits-to-worktree → `git restore` leaked paths in main → resume) shipped clean PRs; without the recovery they would have committed the wrong files to the wrong branch. Always verify isolation up front and pre-commit, and run the orchestrator-level diff check at step 6.
- **Using `git checkout .` or `git clean -fd` to "reset" main during recovery.** Both can wipe unrelated untracked files in the main worktree (e.g. another in-flight WIP, or session-scope draft files like a new skill). Use `git restore <specific paths>` only — target the leaked paths explicitly.

## Related skills and memories

- **`psy-ticket`** — ticket *creation* (this skill is for ticket *execution*).
- **`linear-cli`** — generic Linear CLI surface; drop down to it if `linear issue update --state` lacks a flag you need.
- **`simplify`** — invoked by every dispatched agent before opening its PR.
- `feedback_simplify_before_pr.md` — `/simplify` runs before every PR (single-ticket or batched).
- `feedback_no_speculative_implementation.md` — when a ticket is ambiguous about WHAT to build, STOP and ask.
- `feedback_plan_mode_questions_first.md` — surface forks via `AskUserQuestion` before exiting plan mode / dispatching.
- `feedback_code_complete.md` — manage complexity, plan before coding, decompose big changes.
- `feedback_verify_before_push.md` — verify the fix actually fixes the thing before pushing.
