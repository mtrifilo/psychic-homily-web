---
name: linear-reference
description: Reference for the `linear` CLI (workspace-agnostic). Use whenever a session needs to operate on Linear — read issues, post comments, file projects/milestones/documents, create project-update status posts — beyond simple ticket creation. Workspace-specific conventions (PSY labels, priority scale, branch naming) live in `psy-ticket`; this skill is the underlying CLI surface they sit on.
argument-hint: "[issue|project|project-update|comment|milestone|initiative-update|document|label] [subcommand]"
---

# linear-reference: workspace-agnostic Linear CLI cheat sheet

A reference, not a workflow. Encodes the **command shapes + non-obvious quirks** of the `linear` CLI (v1.11.x verified) so a session doesn't burn turns on `--help` lookups. Project-flavored skills (`psy-ticket`, `psy-solo`, `psy-dispatch`) embed PSY conventions on top of this — they reference this skill rather than re-document the CLI surface.

## Prerequisites

```bash
linear --version                  # 1.11.x verified
which linear                      # /opt/homebrew/bin/linear on this machine
test -f .linear.toml && echo ok   # workspace + team_id pinned per-project
```

The repo's `.linear.toml` pins workspace + team — every subcommand below assumes you're inside the project root so the CLI picks it up automatically. From outside the project, pass `--team <KEY>` and `-w <workspace-slug>` explicitly.

## Top-level command map

| Command | Aliases | Purpose |
|---|---|---|
| `issue` | `i` | Issues — read, create, update, comment |
| `project` | `p` | Projects — list, view, create, update |
| `project-update` | `pu` | Project **status posts** (the timeline updates with health flag) |
| `milestone` | `m` | Project milestones |
| `initiative` | `init` | Initiatives (group of projects) |
| `initiative-update` | `iu` | Initiative status posts (different surface from project-update) |
| `cycle` | `cy` | Team cycles (sprints) |
| `label` | `l` | Workspace labels |
| `document` | `docs`, `doc` | Linear documents (rich-text knowledge base) |
| `team` | `t` | Teams + team config |
| `auth` |  | CLI authentication |

## Universal patterns

### Body content: `--body-file` over `--body`

Almost every "create" or "post" command supports `--body <inline>` AND `--body-file <path>`. Always prefer `--body-file /tmp/<topic>.md`:

- Markdown formatting is preserved without bash-escaping pain.
- The file is a real artifact other tools (like `/psy-self-review`) can read.
- Multiline content with backticks / em-dashes / code fences works first try.

```bash
# Write the body first:
cat <<'EOF' > /tmp/my-body.md
## Heading

- Bullet
- Bullet
EOF

# Then reference it:
linear <command> --body-file /tmp/my-body.md ...
```

### Non-interactive mode: `--no-interactive` (most surfaces, NOT all)

| Surface | Supports `--no-interactive`? |
|---|---|
| `issue create` | ✓ |
| `issue update` | ✓ |
| `project create` | ✓ |
| `project update` (the metadata-edit subcommand) | ✓ |
| `project-update create` (status post) | ✓ |
| `issue comment add` | **✗** — rejects the flag |
| `document create` | ✓ |
| `milestone create` | ✓ |

When in doubt, run `linear <surface> --help` once; record the exception in this file if you find one. Issue-comment is the notable footgun.

### IDs vs slugs

Most commands accept the human-readable form:

- **Issues**: `PSY-823` (full identifier) or just the number.
- **Projects**: pass the *slug-ID* (e.g. `592a829a6dbb`) — the trailing hex segment of the project's URL. Find it via `linear project list` → first column.
- **Milestones**: numeric ID; `linear milestone list --project <slugId>` to enumerate.
- **Labels**: name OR ID; the CLI resolves by name.

## Issues

### View / list

```bash
linear issue view PSY-823                          # human render
linear issue view PSY-823 --json                   # pipeable
linear issue list --sort priority --all-states --all-assignees
linear issue list --project "Project Name" --all-states
linear issue list --label "frontend" --label "Improvement"
```

### Create

```bash
linear issue create \
  --team PSY \
  --title "Short imperative title (under ~80 chars)" \
  --label frontend --label Improvement \
  --priority 3 \
  --project "Project Name" \
  --description-file /tmp/my-ticket-body.md \
  --no-interactive
```

`--description-file` (not `--body-file`) on create — note the asymmetric flag name vs. comment/update.

### Update

```bash
linear issue update PSY-823 --state "In Progress"    # case-sensitive; exact state name
linear issue update PSY-823 --add-label "dogfooded"
linear issue update PSY-823 --remove-label "Bug"
linear issue update PSY-823 --priority 2
linear issue update PSY-823 --assignee me
```

The state name is the **display name** of the workflow state, including spaces. `linear team view <KEY> --json` lists the workflow states for a team.

### Comment

```bash
linear issue comment add PSY-823 --body-file /tmp/comment.md
# WARNING: do NOT pass --no-interactive — this surface rejects the flag.
```

## Projects

### List / view

```bash
linear project list                                  # shows slug-IDs in column 1
linear project list --team PSY
linear project view <slugId>                         # detail render
linear project view <slugId> --json                  # pipeable
```

### Create

```bash
linear project create \
  --team PSY \
  --name "Project Name — YYYY-MM" \
  --description-file /tmp/project-description.md \
  --no-interactive
```

Returns the new project's slug-ID. Capture it for follow-up commands.

### Edit metadata

```bash
linear project update <slugId> --name "New name"
linear project update <slugId> --state "In Progress"
linear project update <slugId> --description-file /tmp/new-desc.md
```

## Project status updates (timeline posts)

Distinct surface from `project update` (metadata edit). These are the **status reports** that show up in a project's activity tab.

### Create

```bash
linear project-update create <slugId> \
  --body-file /tmp/status-update.md \
  --health onTrack \
  --no-interactive
```

**Health values** (exact strings):
- `onTrack`
- `atRisk`
- `offTrack`

Default health is `onTrack` when omitted. Pick the actual signal — these flow into project-health dashboards.

### List

```bash
linear project-update list <slugId>
linear project-update list <slugId> --json
```

## Milestones

```bash
linear milestone list --project <slugId>
linear milestone create \
  --project <slugId> \
  --name "Milestone name" \
  --target-date 2026-06-15
linear milestone update <id> --name "..." --status "in_progress"
```

Target date is `YYYY-MM-DD`. The milestone status enum is lowercase + underscore-separated: `planned`, `in_progress`, `completed`, `canceled`.

## Initiatives

```bash
linear initiative list
linear initiative view <slugId>
linear initiative-update create <initiative-slugId> \
  --body-file /tmp/initiative-update.md \
  --health onTrack
```

Initiative-update is a *different* surface from project-update — projects belong to initiatives, but each has its own timeline. Posting an initiative-update doesn't roll up project-updates.

## Documents

```bash
linear document list
linear document view <slugId>
linear document create \
  --title "Doc title" \
  --content-file /tmp/doc-content.md \
  --project <slugId>          # optional: scope to a project
```

`--content-file` for docs (not `--body-file`), `--description-file` for issues — the flag-name family is **inconsistent across surfaces**. Reference the per-command `--help` if in doubt.

## Labels

```bash
linear label list                          # team-scoped if .linear.toml pins team
linear label list --team PSY
linear label create --name "label-name" --color "#FF0000"
```

Label groups are NOT directly addressable via CLI in v1.11.x — they're a Web-UI concept. Create labels individually and assign them on tickets.

## Common gotchas

- **`--no-interactive` on `issue comment add`**: rejected. Pass only `--body-file`.
- **State names are case-sensitive**: `"In Progress"` not `"in progress"`.
- **`--description-file` vs `--body-file` vs `--content-file`**: inconsistent across `issue create` / `project-update create` / `document create`. Run `--help` once per new surface.
- **Project slug-ID is the hex tail** of the project URL (`/project/name-and-hex` → grab everything after the last `-`). Or `linear project list` and read column 1.
- **Don't pass `--team` from outside the repo** if it's pinned in `.linear.toml`. Inside the repo, the flag is redundant but harmless.
- **`linear` won't follow stdin for interactive mode in agent sessions** — always pass `--no-interactive` when scripting (except where the surface rejects it).
- **Description / body files persist between commands** — write to `/tmp/<topic>.md` and re-reference. Don't inline multi-paragraph markdown via `--body "..."`.

## Where the workspace conventions live

This skill stays **workspace-agnostic**. Project-specific norms (priority scale meanings, label sets, branch naming, ticket-body templates, when to file a `confidence:*` label) live in:

- `psy-ticket` — Psychic Homily ticket + project creation conventions.
- `psy-solo` — single-ticket ship workflow (issue update + comment + project-update at specific phases).
- `psy-dispatch` — parallel-worktree batch workflow.

Load `psy-ticket` whenever you need PSY-flavored Linear work (label sets, confidence labels, branch naming). Load this skill (`linear-reference`) underneath whenever you need a CLI shape this skill documents and `psy-ticket` doesn't.
