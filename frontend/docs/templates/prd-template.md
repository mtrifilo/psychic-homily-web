# [Feature Name]

> **Instructions:** Copy this template to create a new PRD. Replace bracketed placeholders with your content. Delete this instruction block and any helper text (in blockquotes) once complete.

## Summary

> One to three sentences describing what this feature does and why it matters.

[Brief description of the feature and its primary value proposition]

**Status:** Planning | In Progress | Completed
**Priority:** High | Medium | Low
**Estimated Scope:** Small (hours) | Medium (1-2 days) | Large (3+ days)

---

## Problem Statement

> What problem does this solve? Why is this feature needed now?

### Current State
[Describe how things work today, or what's missing]

### Pain Points
- [Pain point 1]
- [Pain point 2]

### Success Criteria
> How will we know this feature is successful?

- [ ] [Measurable outcome 1]
- [ ] [Measurable outcome 2]

---

## User Stories

> Who benefits from this feature and how?

1. **As a [user type]**, I want to [action] so that [benefit]
2. **As a [user type]**, I want to [action] so that [benefit]

---

## Requirements

### Functional Requirements

> What must this feature do? Be specific.

#### Must Have (MVP)
- [ ] [Core requirement 1]
- [ ] [Core requirement 2]

#### Should Have
- [ ] [Important but not critical for launch]

#### Could Have (Future)
- [ ] [Nice-to-have for later iterations]

### Non-Functional Requirements

> Performance, security, accessibility considerations.

- **Performance:** [e.g., Page load under 2 seconds]
- **Accessibility:** [e.g., Keyboard navigable, screen reader compatible]
- **Security:** [e.g., Admin-only access, input validation]

---

## Technical Context

> Provide context that helps understand the existing system.

### Relevant Codebase Areas

> List files, directories, or patterns Claude should examine.

**Backend:**
- `backend/internal/api/handlers/[relevant].go` - [what it does]
- `backend/internal/services/[relevant].go` - [what it does]

**Frontend:**
- `frontend/app/[route]/page.tsx` - [what it does]
- `frontend/components/[component].tsx` - [what it does]

**Database:**
- Table: `[table_name]` - [what it stores]

### Existing Patterns to Follow

> Point to examples of similar implementations in the codebase.

- For API endpoints, see: `[example file]`
- For React components, see: `[example file]`
- For hooks, see: `[example file]`

### Dependencies

> External services, libraries, or APIs involved.

- [Dependency 1]: [how it's used]
- [Dependency 2]: [how it's used]

---

## Proposed Solution

> High-level description of the approach. Claude can help refine this.

### Overview

[Describe the general approach to solving this problem]

### Key Components

1. **[Component 1]:** [Brief description]
2. **[Component 2]:** [Brief description]

### Data Flow

> Optional: Describe how data moves through the system.

```
[User Action] → [Frontend] → [API] → [Backend Service] → [Database]
                    ↓
              [Response] → [UI Update]
```

---

## API Design

> Define new or modified endpoints. Skip if not applicable.

### New Endpoints

#### `[METHOD] /[path]`

**Description:** [What this endpoint does]

**Request:**
```json
{
  "field": "value"
}
```

**Response:**
```json
{
  "field": "value"
}
```

**Error Responses:**
| Status | Condition |
|--------|-----------|
| 400 | [When this happens] |
| 404 | [When this happens] |

---

## UI/UX Design

> Describe the user interface. Text mockups work well.

### Wireframe

```
┌─────────────────────────────────────────┐
│  [Header / Navigation]                  │
├─────────────────────────────────────────┤
│                                         │
│  [Main content area]                    │
│                                         │
│  ┌─────────────┐  ┌─────────────┐       │
│  │  Component  │  │  Component  │       │
│  └─────────────┘  └─────────────┘       │
│                                         │
│  [Action Button]                        │
│                                         │
└─────────────────────────────────────────┘
```

### User Flow

1. User navigates to [location]
2. User sees [initial state]
3. User [takes action]
4. System [responds]
5. User sees [result]

### States

- **Loading:** [How loading appears]
- **Empty:** [What shows when no data]
- **Error:** [How errors are displayed]
- **Success:** [Confirmation feedback]

---

## Database Changes

> Skip if no schema changes needed.

### New Tables

```sql
CREATE TABLE [table_name] (
    id BIGSERIAL PRIMARY KEY,
    [field_name] [TYPE] [CONSTRAINTS],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Schema Modifications

```sql
ALTER TABLE [existing_table] ADD COLUMN [column_name] [TYPE];
```

### Migration Notes

- [Any data migration considerations]
- [Rollback strategy]

---

## Implementation Notes

> Guidance for implementation. Add constraints, gotchas, or preferences.

### Approach Preferences

- [Prefer X approach over Y because...]
- [Use existing pattern from Z]
- [Avoid X because...]

### Known Challenges

- [Challenge 1]: [Potential solution or consideration]
- [Challenge 2]: [Potential solution or consideration]

### Testing Strategy

- [ ] Unit tests for [what]
- [ ] Integration tests for [what]
- [ ] Manual testing for [what]

---

## Out of Scope

> Explicitly state what this feature does NOT include to prevent scope creep.

- [Thing that might seem related but isn't included]
- [Future enhancement that's not part of this work]

---

## Open Questions

> Unresolved decisions that may need input.

1. [Question about approach or requirement]
2. [Question about edge case handling]

---

## References

> Links to related documents, designs, or external resources.

- [Related PRD or design doc](./path-to-doc.md)
- [External API documentation](https://example.com/docs)
- [Inspiration or reference](https://example.com)

---

## Changelog

| Date | Author | Changes |
|------|--------|---------|
| YYYY-MM-DD | [Name] | Initial draft |
