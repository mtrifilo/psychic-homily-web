import { lexer, type Token } from 'marked'

const DEFAULT_MAX_LENGTH = 200
const ELLIPSIS = '...'

/**
 * Top-level token types that introduce a block-level boundary. When we flatten
 * the markdown into a single line, the text on either side of one of these must
 * not run together (e.g. a heading immediately followed by a paragraph), so we
 * emit a separating space. (List items are separated inside the `list` branch,
 * since marked never emits a bare top-level `list_item`.)
 */
const BLOCK_BOUNDARY_TYPES = new Set([
  'paragraph',
  'heading',
  'blockquote',
  'list',
  'code',
  'space',
  'br',
  'hr',
])

/**
 * Named HTML entities markdown processors emit (the inverse of marked's own
 * escape table). marked's lexer leaves these encoded in text tokens — decoding
 * only happens at render time — so we decode them here. `&#39;` is handled by
 * the numeric branch of `decodeEntities`, so it is intentionally absent.
 */
const NAMED_ENTITIES: Record<string, string> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: ' ',
}

/**
 * Decode the bounded set of HTML entities that can appear in lexer text
 * tokens: the named entities markdown emits, plus the fully general numeric
 * forms (`&#160;` / `&#xA0;`). Anything outside this set is left untouched —
 * this is intentionally not a full named-entity table, because that input
 * space does not occur in the markdown/MDX content this util processes.
 */
function decodeEntities(text: string): string {
  if (!text.includes('&')) return text
  return text.replace(/&(#x?[0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*);/g, (match, body) => {
    if (body[0] === '#') {
      const codePoint =
        body[1] === 'x' || body[1] === 'X'
          ? parseInt(body.slice(2), 16)
          : parseInt(body.slice(1), 10)
      if (Number.isNaN(codePoint)) return match
      try {
        return String.fromCodePoint(codePoint)
      } catch {
        return match
      }
    }
    const named = NAMED_ENTITIES[body]
    return named ?? match
  })
}

/**
 * Collect the human-readable text out of a single marked token, recursing into
 * child tokens where present.
 *
 * Why walk tokens instead of reading `token.text`? For container tokens
 * (paragraph, heading, strong, em, link, …) `token.text` is the *raw markdown
 * source* of their contents, which still includes link syntax, emphasis
 * markers, and HTML. The child `tokens` array is the already-parsed inner
 * content, so recursing yields true plain text. Leaf tokens (codespan, fenced
 * code, escape, and text without children) carry decoded text directly —
 * marked has already turned `&amp;` into `&` and `\*` into `*` for us.
 */
function collectText(token: Token, out: string[]): void {
  switch (token.type) {
    // Inline + block HTML (including self-closing MDX/JSX components such as
    // <Bandcamp .../>): drop the tag entirely. Any human-readable text that
    // sits between HTML tags is parsed into its own sibling text tokens, so
    // dropping the tag here strips markup without swallowing prose.
    case 'html':
    case 'image':
    case 'def':
    case 'br':
    case 'hr':
    case 'space':
      break

    // Leaf tokens whose text we keep. Escaped sequences (\* \#) and code spans
    // are literal already; entity decoding turns &amp; into & for prose.
    case 'codespan':
    case 'escape':
    case 'code':
      out.push(decodeEntities(token.text))
      break

    // `text` tokens are leaves *unless* they were re-tokenized (e.g. inside a
    // loose list item), in which case the children hold the real content.
    case 'text':
      if ('tokens' in token && token.tokens && token.tokens.length > 0) {
        for (const child of token.tokens) collectText(child, out)
      } else {
        out.push(decodeEntities(token.text))
      }
      break

    // List is a container of list items; separate each item so adjacent items
    // don't fuse into one word once whitespace is collapsed.
    case 'list':
      for (const item of token.items) {
        const before = out.length
        collectText(item, out)
        if (out.length > before) out.push(' ')
      }
      break

    // Container tokens: recurse into the parsed inner content.
    default:
      if ('tokens' in token && token.tokens) {
        for (const child of token.tokens) collectText(child, out)
      }
      break
  }
}

/**
 * Convert a markdown (or MDX) string into a single line of plain text.
 *
 * Markdown formatting, links, inline/block HTML, and self-closing JSX embeds
 * are stripped; encoded entities are decoded; whitespace is collapsed. Use this
 * for excerpts, meta descriptions, or anywhere markdown source would otherwise
 * leak into a plain-text context.
 */
export function markdownToPlainText(markdown: string): string {
  if (!markdown) return ''

  const tokens = lexer(markdown)
  const parts: string[] = []

  for (const token of tokens) {
    const before = parts.length
    collectText(token, parts)
    // Insert a separator after a block-level token so its text doesn't fuse
    // with the next block's text once whitespace is collapsed.
    if (parts.length > before && BLOCK_BOUNDARY_TYPES.has(token.type)) {
      parts.push(' ')
    }
  }

  return parts.join('').replace(/\s+/g, ' ').trim()
}

/**
 * Produce a plain-text excerpt from markdown/MDX content, truncated to
 * `maxLength` characters with a trailing ellipsis when truncation occurs.
 *
 * Truncation is measured against the rendered plain text, not the raw markdown,
 * so link/HTML syntax never inflates or leaks into the excerpt length.
 */
export function getTextExcerpt(
  content: string,
  maxLength = DEFAULT_MAX_LENGTH
): string {
  const text = markdownToPlainText(content)
  if (text.length <= maxLength) return text
  return text.slice(0, maxLength).trim() + ELLIPSIS
}
