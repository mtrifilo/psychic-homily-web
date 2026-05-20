import { describe, it, expect } from 'vitest'
import { markdownToPlainText, getTextExcerpt } from './markdownExcerpt'

describe('markdownToPlainText', () => {
  it('returns empty string for empty/whitespace input', () => {
    expect(markdownToPlainText('')).toBe('')
    expect(markdownToPlainText('   \n  ')).toBe('')
  })

  it('strips inline emphasis and code markers', () => {
    expect(markdownToPlainText('This is **bold** and _italic_ text.')).toBe(
      'This is bold and italic text.'
    )
    expect(markdownToPlainText('Run `npm install` first.')).toBe(
      'Run npm install first.'
    )
  })

  it('flattens deeply nested markdown to plain text', () => {
    expect(
      markdownToPlainText('This is **bold with _nested italic_ and `code`** here.')
    ).toBe('This is bold with nested italic and code here.')
  })

  it('keeps link text but drops the URL and bracket syntax', () => {
    expect(
      markdownToPlainText('See [the docs](https://example.com/path?q=1) now.')
    ).toBe('See the docs now.')
  })

  it('strips MDX/JSX embed components such as <Bandcamp .../>', () => {
    const content =
      '<Bandcamp album="123" artist="x" title="y" />\n\n[Winter](https://example.com) and Hooky have a new EP called _Water Season_ out now.'
    expect(markdownToPlainText(content)).toBe(
      'Winter and Hooky have a new EP called Water Season out now.'
    )
  })

  it('strips inline HTML tags but preserves the inner text', () => {
    expect(
      markdownToPlainText('Some <strong>raw HTML</strong> and <a href="x">a link</a> in text.')
    ).toBe('Some raw HTML and a link in text.')
  })

  it('drops block-level HTML', () => {
    expect(
      markdownToPlainText('<div class="x">block html</div>\n\nReal paragraph.')
    ).toBe('Real paragraph.')
  })

  it('decodes named HTML entities', () => {
    expect(
      markdownToPlainText('Tom &amp; Jerry &lt;3 &quot;quotes&quot; and AT&amp;T.')
    ).toBe('Tom & Jerry <3 "quotes" and AT&T.')
  })

  it('decodes numeric and hex HTML entities', () => {
    expect(markdownToPlainText('Caf&#233; &#x2014; end.')).toBe('Café — end.')
  })

  it('leaves unknown entities untouched', () => {
    expect(markdownToPlainText('Keep &unknownentity; intact.')).toBe(
      'Keep &unknownentity; intact.'
    )
  })

  it('preserves backslash-escaped markdown characters as literals', () => {
    expect(
      markdownToPlainText('Literal \\*asterisks\\* and \\#hash should stay.')
    ).toBe('Literal *asterisks* and #hash should stay.')
  })

  it('separates block-level content with a single space', () => {
    expect(markdownToPlainText('# Title\nBody paragraph here.')).toBe(
      'Title Body paragraph here.'
    )
    expect(markdownToPlainText('> Quoted **wisdom**.\n\nFollow-up.')).toBe(
      'Quoted wisdom. Follow-up.'
    )
  })

  it('separates list items so they do not fuse together', () => {
    expect(markdownToPlainText('- one\n- two\n- three')).toBe('one two three')
  })

  it('collapses runs of whitespace into single spaces', () => {
    expect(markdownToPlainText('a\t\t\tb     c\nd')).toBe('a b c d')
  })
})

describe('getTextExcerpt', () => {
  it('returns the full plain text when under the limit', () => {
    expect(getTextExcerpt('Short **post**.', 200)).toBe('Short post.')
  })

  it('truncates and appends an ellipsis when over the limit', () => {
    const long = 'word '.repeat(100)
    const result = getTextExcerpt(long, 50)
    expect(result.endsWith('...')).toBe(true)
    // 50-char slice, trailing space trimmed, then '...' appended.
    expect(result.length).toBeLessThanOrEqual(53)
    expect(result.startsWith('word word')).toBe(true)
  })

  it('does not append an ellipsis when text length equals the limit', () => {
    const text = 'a'.repeat(10)
    expect(getTextExcerpt(text, 10)).toBe(text)
  })

  it('defaults to a 200-character limit', () => {
    const long = 'x'.repeat(500)
    const result = getTextExcerpt(long)
    expect(result.length).toBe(203) // 200 chars + '...'
  })

  it('truncates against the plain text, not the raw markdown length', () => {
    // 60 chars of link markdown collapse to 8 chars of visible text ("the docs"),
    // which is under the limit — so no truncation despite long raw source.
    const content = '[the docs](https://example.com/a/very/long/url/that/keeps/going)'
    expect(getTextExcerpt(content, 20)).toBe('the docs')
  })

  it('returns empty string for empty input', () => {
    expect(getTextExcerpt('', 100)).toBe('')
  })
})
