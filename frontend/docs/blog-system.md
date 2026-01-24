# Blog System Documentation

This document explains how to write and publish new blog posts on Psychic Homily, along with an overview of the blog system architecture.

## Quick Start: Creating a New Blog Post

### Using the CLI (Recommended)

The easiest way to create a new blog post is with the CLI tool:

```bash
npm run blog:new
# or
bun run blog:new
```

The CLI will:
1. Prompt you for a title
2. Let you select from existing categories or create new ones
3. Ask for a description (for SEO)
4. Generate the file with proper naming and frontmatter
5. Open the file in your editor (VS Code by default)

**Example session:**
```
╔════════════════════════════════════════╗
║     📝 New Blog Post Generator         ║
╚════════════════════════════════════════╝

[1/4] What's the title of your post?
Title: New Release: Album Name by Artist

[2/4] Select categories for your post

Existing categories:
  1. Arizona Artists
  2. New Release
  3. Psychic Homily
  0. Enter new category

Enter numbers separated by commas, or 0 for new: 2

[3/4] Add a short description (for SEO & previews)
Description: A review of the latest album...

[4/4] Creating your blog post...

════════════════════════════════════════
  ✅ Blog post created successfully!
════════════════════════════════════════

  📄 File: 2025-01-24-new-release-album-name-by-artist.md
  🏷️  Categories: New Release

Open in code? (Y/n):
```

To use a different editor, set the `EDITOR` environment variable:
```bash
EDITOR=vim npm run blog:new
```

---

### Manual Method

If you prefer to create files manually:

#### 1. Create a New Markdown File

Create a new `.md` file in the `content/blog/` directory. Use the naming convention:

```
YYYY-MM-DD-url-slug.md
```

Example: `2025-03-15-new-album-review.md`

### 2. Add Frontmatter

Every blog post requires YAML frontmatter at the top of the file:

```yaml
---
title: "Your Post Title"
date: 2025-03-15
categories: ["New Release"]
description: "A brief description for SEO and social sharing"
---
```

| Field | Required | Description |
|-------|----------|-------------|
| `title` | Yes | The display title of the post |
| `date` | Yes | Publication date (YYYY-MM-DD or ISO 8601 format) |
| `categories` | No | Array of category names (e.g., `["New Release", "Show Review"]`) |
| `description` | No | Short description for meta tags and excerpts |

### 3. Write Your Content

After the frontmatter, write your post content using Markdown:

```markdown
---
title: "New Release: Album Name by Artist"
date: 2025-03-15
categories: ["New Release"]
description: "Review of the new album..."
---

This is the first paragraph of your blog post.

## Subheading

More content here with **bold** and *italic* text.

[Link text](https://example.com)
```

### 4. Adding Media Embeds

The blog supports two embed types for music:

#### Bandcamp Embed

```markdown
<Bandcamp album="ALBUM_ID" artist="ARTIST_SLUG" title="ALBUM_SLUG" />
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| `album` | Yes* | Bandcamp album ID (found in embed code) |
| `track` | Yes* | Bandcamp track ID (use instead of album for single tracks) |
| `artist` | Yes | Artist's Bandcamp subdomain (e.g., `daydreamingwinter`) |
| `title` | Yes | Album/track slug from the URL |
| `size` | No | `large` (default) or `small` |
| `artwork` | No | `small` (default) or `big` |
| `height` | No | Embed height in pixels (default: `120`) |
| `tracklist` | No | `true` or `false` (default) |

*Either `album` or `track` is required, not both.

**Example:**
```markdown
<Bandcamp album="3441665372" artist="daydreamingwinter" title="water-season" />
```

**Finding the Album ID:**
1. Go to the album page on Bandcamp
2. Click "Share / Embed"
3. Look for `album=XXXXXXX` in the embed code

#### SoundCloud Embed

```markdown
<SoundCloud url="EMBED_URL" title="Track Title" artist="Artist Name" />
```

| Attribute | Required | Description |
|-----------|----------|-------------|
| `url` | Yes | Full SoundCloud embed URL |
| `title` | No | Track title for accessibility |
| `artist` | No | Artist name |
| `artist_url` | No | Link to artist's SoundCloud |
| `track_url` | No | Link to the track |

### 5. Publish

Blog posts are automatically published when:
1. The file is saved to `content/blog/`
2. The filename does NOT start with `_` (underscore prefix marks drafts)
3. The site is rebuilt/deployed

For local development:
```bash
npm run dev
```

For production, push to the main branch to trigger deployment.

---

## Architecture Overview

### Directory Structure

```
frontend/
├── content/
│   └── blog/                    # Blog post markdown files
│       ├── _index.md            # (ignored - can be used for drafts)
│       └── YYYY-MM-DD-slug.md   # Blog posts
├── scripts/
│   └── new-blog-post.ts         # CLI tool for creating posts
├── lib/
│   ├── blog.ts                  # Core blog utilities
│   └── types/
│       └── blog.ts              # TypeScript types
├── app/
│   └── blog/
│       ├── page.tsx             # Blog listing page (/blog)
│       └── [slug]/
│           └── page.tsx         # Individual post page (/blog/slug)
└── components/
    └── blog/
        ├── mdx-content.tsx      # MDX renderer with custom components
        ├── bandcamp-embed.tsx   # Bandcamp player component
        └── soundcloud-embed.tsx # SoundCloud player component
```

### How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                     Build/Request Time                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  content/blog/*.md                                              │
│         │                                                       │
│         ▼                                                       │
│  ┌─────────────────┐                                            │
│  │   lib/blog.ts   │  Reads markdown files                      │
│  │                 │  Parses frontmatter (gray-matter)          │
│  │  getBlogSlugs() │  Converts shortcodes to MDX components     │
│  │  getBlogPost()  │  Extracts excerpts                         │
│  │  getAllPosts()  │                                            │
│  └────────┬────────┘                                            │
│           │                                                     │
│           ▼                                                     │
│  ┌─────────────────────────────────────────┐                    │
│  │         app/blog/page.tsx               │                    │
│  │         app/blog/[slug]/page.tsx        │                    │
│  │                                         │                    │
│  │  - Static generation (generateStaticParams)                  │
│  │  - SEO metadata (generateMetadata)                           │
│  │  - Renders MDXContent component                              │
│  └────────┬────────────────────────────────┘                    │
│           │                                                     │
│           ▼                                                     │
│  ┌─────────────────────────────────────────┐                    │
│  │    components/blog/mdx-content.tsx      │                    │
│  │                                         │                    │
│  │  - Uses next-mdx-remote/rsc             │                    │
│  │  - Provides custom components:          │                    │
│  │    • <Bandcamp />                       │                    │
│  │    • <SoundCloud />                     │                    │
│  │    • Styled HTML elements               │                    │
│  └─────────────────────────────────────────┘                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Key Components

#### `lib/blog.ts`

Core utilities for blog post management:

| Function | Description |
|----------|-------------|
| `getBlogSlugs()` | Returns array of all post slugs (filenames without `.md`) |
| `getBlogPost(slug)` | Fetches and parses a single post by slug |
| `getAllBlogPosts()` | Returns metadata for all posts, sorted by date |
| `getAllCategories()` | Returns unique categories across all posts |
| `getCategorySlug(name)` | Converts category name to URL-safe slug |
| `getPostsByCategory(slug)` | Filters posts by category |

#### `lib/types/blog.ts`

TypeScript interfaces:

```typescript
interface BlogPostFrontmatter {
  title: string
  date: string
  categories?: string[]
  description?: string
}

interface BlogPost {
  slug: string
  frontmatter: BlogPostFrontmatter
  content: string      // MDX-ready content
  excerpt: string      // Plain text excerpt
}

interface BlogPostMeta {
  slug: string
  title: string
  date: string
  categories: string[]
  description?: string
  excerpt: string
}
```

#### `components/blog/mdx-content.tsx`

Renders MDX content with:
- Custom embed components (`<Bandcamp />`, `<SoundCloud />`)
- Styled HTML elements (headings, paragraphs, links, lists, etc.)
- External link handling (opens in new tab)

### Static Generation

The blog uses Next.js static generation for optimal performance:

1. **`generateStaticParams()`** - Pre-renders all blog post pages at build time
2. **`generateMetadata()`** - Generates SEO metadata for each post
3. Content is read from the filesystem, not a database

### File Naming Convention

Files prefixed with `_` (underscore) are ignored:
- `_draft-post.md` - Will NOT be published
- `2025-03-15-published-post.md` - Will be published

### Date Sorting

Posts are automatically sorted by date (newest first) in:
- Blog listing page
- Category pages
- `getAllBlogPosts()` function

---

## Examples

### Basic Text Post

```markdown
---
title: "Show Recap: Band Name at Venue"
date: 2025-03-20
categories: ["Show Review"]
description: "Recap of last night's incredible show"
---

Last night's show was unforgettable. The energy in the room was electric from the moment the first note hit.

## The Opening Act

The night kicked off with a local favorite...

## Main Event

When the headliner took the stage...
```

### Album Review with Bandcamp Embed

```markdown
---
title: "New Release: Water Season by Winter and Hooky"
date: 2025-02-17
categories: ["New Release"]
description: "Water Season by Winter and Hooky on cassette"
---

<Bandcamp album="3441665372" artist="daydreamingwinter" title="water-season" />

[Winter](https://instagram.com/daydreamingwinter) and [Hooky](https://instagram.com/h0o0ky) have a new collaborative EP called _Water Season_ out now. Thick with playful effects and recording experiments, this EP captures something special...
```

### Multiple Categories

```markdown
---
title: "Local Band Releases Debut Album & Announces Tour"
date: 2025-04-01
categories: ["New Release", "Tour Announcement", "Local Artist"]
description: "Exciting news from the local scene"
---

Big news today...
```

---

## Troubleshooting

### Post Not Appearing

1. Check filename doesn't start with `_`
2. Verify frontmatter is valid YAML (check for missing quotes, colons)
3. Ensure date is in valid format
4. Restart dev server if running locally

### Embed Not Rendering

1. Verify component syntax uses self-closing tag: `<Bandcamp ... />`
2. Check all required attributes are present
3. Ensure album/track ID is correct

### Build Errors

1. Check for syntax errors in frontmatter
2. Verify all required frontmatter fields are present
3. Check for unclosed MDX tags

---

## Future Enhancements

Potential improvements to consider:
- Draft preview functionality
- Scheduled publishing
- Image optimization
- RSS feed generation
- Related posts
- Reading time estimates
