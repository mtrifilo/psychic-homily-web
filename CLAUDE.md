# Psychic Homily — agent primer

Project-root CLAUDE.md (gitignored). Loaded automatically by Claude Code when working in this repo. Curated, terse, high-signal — not a substitute for the deeper auto-memory system.

## Where the deep project knowledge lives

The full project-knowledge index is the **user-level auto-memory** at:

```
~/.claude/projects/-Users-mtrifilo-dev-psychic-homily-web/memory/MEMORY.md
```

Read that for: completed phases, key non-obvious patterns, discovery pipeline strategy, radio entities architecture, workflow, feedback memories, references. Topic files (`pattern_*.md`, `feedback_*.md`, `project_*.md`) live alongside `MEMORY.md` in the same dir.

When the user says "remember X", that auto-memory system writes there — not here. This file is hand-curated. They're complementary, not duplicates.

## Read-this-first conventions

- **One issue = one PR.** Branch naming: `PSY-{N}/kebab-description`. PR body must include `Closes PSY-{N}`.
- **Tests + manual repro before any push.** Every PR has `/simplify` + relevant local tests run first; failure blocks push (escalate to orchestrator instead of pushing past it). See `feedback_simplify_before_pr.md` and `pattern_dispatch_stacks.md`.
- **No speculative implementation.** When a ticket is ambiguous about WHAT to build (categories, thresholds, taxonomies, UX), STOP and ask. See `feedback_no_speculative_implementation.md`.
- **Audit docs are point-in-time, not authoritative.** If a ticket cites `docs/research/*.md`, re-verify counts/sites against current code before relying on them.

## Critical project paths

- `backend/` — Go (Huma + GORM). Server entry: `cmd/server`. Migrations: `backend/db/migrations/`.
- `frontend/` — Next.js. Component features at `frontend/features/<area>/`. Shared primitives at `frontend/components/shared/`.
- `docs/` — gitignored, agent-only. Entry point: `docs/INDEX.md`. Buckets: `runbooks/`, `features/`, `open-questions/`, `research/`, `vision.md`.
- `.claude/skills/` — project-specific agent skills (`psy-dispatch`, `psy-ticket`).
- `scripts/dispatch/` — per-worktree dev stack helpers (`stack-up.sh`, `stack-down.sh`, `stack-cleanup.sh`). See `pattern_dispatch_stacks.md` for the 3-tier mode model (PSY-624).

## Mutation feedback

- **Every mutation hook (`useMutation` or equivalent) MUST expose an error state**, and the consuming component MUST surface it via the inline-banner pattern. Don't swallow 4xx/5xx — silent failures were the project-wide audit finding behind PSY-596.
- **Canonical primitive:** `frontend/components/shared/InlineErrorBanner.tsx` (PSY-623, with `queryFallback` variant added in PSY-630). Use it for non-optimistic mutations (create / update / delete / permission); reach for the auto-dismiss `useAutoDismissError` hook for optimistic-rollback shapes (vote/like/reorder). Full sticky-vs-auto-dismiss policy + audit-umbrella status in `pattern_mutation_feedback.md`.
- **Canonical PR examples:** PSY-608 (comments + field notes), PSY-609 (collections), PSY-610 (tag-admin), PSY-630 (cross-feature sweep + queryFallback). New mutation surfaces should mirror one of these — no new toast library, no parallel error-banner primitive.

## How to run things

```bash
# Backend
cd backend && go test ./<package>/...       # scoped tests
cd backend && go build ./...                # whole-graph compile check
cd backend && bash run-dev.sh               # local dev (postgres + migrate + server)

# Frontend
cd frontend && bun run typecheck            # tsc --noEmit
cd frontend && bun run test:run <scope>     # scoped vitest (e.g. test:run features/comments)
cd frontend && bun run dev                  # Next.js dev server on :3000

# Per-worktree dispatch stack (frontend behavioral tickets in agent worktrees)
bash scripts/dispatch/stack-up.sh "$(git rev-parse --show-toplevel)" --mode={none,shared,isolated}
bash scripts/dispatch/stack-down.sh "$(git rev-parse --show-toplevel)"
bash scripts/dispatch/stack-cleanup.sh [--dry-run]
```

## Active project (May 2026)

"Entity & Collections Dogfood — May 2026" Linear project — see `linear issue list --project "Entity & Collections Dogfood — May 2026" --all-states --all-assignees`. Current focus: closing dogfood-pass items (PSY-577..617 range) and follow-up infrastructure (PSY-621..624).

## Linear CLI essentials

```bash
linear --version  # 1.11.x verified
test -f .linear.toml && echo ok  # team_id=PSY pinned
linear issue list --sort priority --team PSY --all-states --all-assignees
linear issue create --team PSY --title "..." --label backend --label Improvement --priority 3 \
  --project "..." --description-file /tmp/desc.md --no-interactive
linear issue update PSY-XXX --state "In Progress"  # case-sensitive, use exact name
linear issue comment add PSY-XXX --body-file /tmp/body.md  # NO --no-interactive flag
```

## When to consult what

| Question | Source |
| --- | --- |
| "What does this codebase look like?" | Read the code; this file just orients. |
| "What's the recent project status, what shipped?" | `MEMORY.md` Current status pointer → `project_current_status.md` |
| "Is there a pattern for X?" | `MEMORY.md` Key Non-Obvious Patterns → topic file pointers |
| "Has the user given feedback about X?" | `MEMORY.md` Feedback section → `feedback_*.md` files |
| "How do I dispatch a batch of tickets in worktrees?" | `.claude/skills/psy-dispatch/SKILL.md` |
| "How do I file a Linear ticket?" | `.claude/skills/psy-ticket/SKILL.md` |
| "What's the convention for X (mutation feedback, dispatch stacks, user attribution)?" | `pattern_*.md` topic files in user-level memory dir |
