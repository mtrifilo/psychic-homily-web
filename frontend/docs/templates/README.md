# Document Templates

Templates for planning and documenting features in this codebase. These are designed to provide effective context for AI-assisted development with Claude Code.

## Available Templates

- **[prd-template.md](./prd-template.md)** - Product Requirements Document for new features

## Using the PRD Template with Claude Code

### Why Use a PRD?

A well-written PRD helps Claude Code:
- Understand the *why* behind the feature, not just the *what*
- Find relevant existing code patterns to follow
- Make consistent architectural decisions
- Avoid scope creep by knowing what's out of scope
- Ask better clarifying questions

### Quick Start

1. Copy `prd-template.md` to a new file: `[feature-name]-prd.md`
2. Fill in the sections relevant to your feature (not all sections are required)
3. Point Claude Code to the PRD: "Read the PRD at frontend/docs/[feature-name]-prd.md and implement it"

### Tips for Effective PRDs

**Be specific about what exists:**
```markdown
# Good
For API endpoints, follow the pattern in backend/internal/api/handlers/show.go

# Less helpful
Follow existing patterns in the codebase
```

**Include concrete examples:**
```markdown
# Good
User clicks "Export" → downloads file named "show-2024-03-15-band-name.md"

# Less helpful
User can export shows
```

**State preferences explicitly:**
```markdown
# Good
Use shadcn/ui Button component, not custom buttons

# Less helpful
Use consistent styling
```

**Define boundaries clearly:**
```markdown
# Good
Out of scope: batch export, scheduled exports, export to Google Calendar

# Risky
(no out of scope section - Claude may over-build)
```

### Which Sections to Fill Out

**Always include:**
- Summary
- Problem Statement
- User Stories
- Requirements (at least MVP)
- Technical Context (relevant files)

**Include when applicable:**
- API Design (if new endpoints)
- Database Changes (if schema changes)
- UI/UX Design (if user-facing)

**Optional but helpful:**
- Implementation Notes (gotchas, preferences)
- Open Questions (decisions you want Claude to help with)
- Out of Scope (prevent over-engineering)

### Working with Claude Code

**Starting a feature:**
```
Read frontend/docs/feature-name-prd.md and create an implementation plan
```

**After Claude proposes a plan:**
- Review the plan before approving
- Ask clarifying questions
- Point out any misunderstandings

**During implementation:**
```
Continue implementing the PRD at frontend/docs/feature-name-prd.md
```

**Updating the PRD:**
The PRD is a living document. Update it when:
- Requirements change
- You make implementation decisions
- Open questions get resolved
- You discover new constraints

### Minimal PRD

Not every feature needs a full PRD. For small features, you might only need:

```markdown
# Add Venue Hours Display

## Summary
Display venue operating hours on venue detail pages.

## Requirements
- Show hours on VenueDetail component
- Handle venues without hours (show "Hours not available")
- Format: "Mon-Thu: 4pm-12am, Fri-Sat: 4pm-2am"

## Technical Context
- Venue model: backend/internal/models/venue.go (hours field exists, may be empty)
- Venue detail: frontend/components/VenueDetail.tsx

## Out of Scope
- Editing hours (admin feature for later)
- Real-time "Open now" indicator
```
