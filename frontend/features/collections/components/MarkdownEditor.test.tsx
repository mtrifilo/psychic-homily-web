import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { MarkdownEditor, MarkdownContent } from './MarkdownEditor'

describe('MarkdownEditor', () => {
  it('renders the textarea in Write mode by default', () => {
    render(<MarkdownEditor value="" onChange={vi.fn()} />)
    expect(screen.getByTestId('markdown-editor-textarea')).toBeInTheDocument()
    expect(screen.queryByTestId('markdown-editor-preview')).not.toBeInTheDocument()
  })

  it('forwards typed input to the onChange callback', async () => {
    const handleChange = vi.fn()
    const user = userEvent.setup()
    render(<MarkdownEditor value="" onChange={handleChange} />)

    await user.type(screen.getByTestId('markdown-editor-textarea'), 'hi')

    // userEvent fires onChange per-character; assert the most recent value is the
    // last character (controlled component pattern: component never updates value).
    expect(handleChange).toHaveBeenCalled()
    expect(handleChange.mock.calls.at(-1)?.[0]).toBe('i')
  })

  it('toggles to preview when the Preview tab is clicked', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value="**hello**" onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))

    expect(screen.getByTestId('markdown-editor-preview')).toBeInTheDocument()
    expect(screen.queryByTestId('markdown-editor-textarea')).not.toBeInTheDocument()
  })

  it('renders bold markdown as <strong> in the preview', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value="**bold**" onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.querySelector('strong')?.textContent).toBe('bold')
  })

  it('renders italic markdown as <em> in the preview', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value="*italic*" onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.querySelector('em')?.textContent).toBe('italic')
  })

  it('renders link markdown as an <a href>', async () => {
    const user = userEvent.setup()
    render(
      <MarkdownEditor
        value="[click](https://example.com)"
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    const anchor = preview.querySelector('a')
    expect(anchor?.getAttribute('href')).toBe('https://example.com')
    expect(anchor?.textContent).toBe('click')
  })

  it('renders blockquote markdown as <blockquote>', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value="> a quote" onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.querySelector('blockquote')).not.toBeNull()
  })

  it('renders bulleted list markdown as <ul><li>', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value={'- one\n- two'} onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.querySelector('ul')).not.toBeNull()
    expect(preview.querySelectorAll('li').length).toBe(2)
  })

  it('strips <script> tags from the preview', async () => {
    const user = userEvent.setup()
    render(
      <MarkdownEditor
        value="hi <script>alert(1)</script>"
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.querySelector('script')).toBeNull()
    expect(preview.innerHTML).not.toMatch(/<script\b/i)
    // The harmless text content survives; only the executable tag is dropped.
    expect(preview.textContent).toContain('hi')
  })

  it('strips <img onerror> payloads from the preview', async () => {
    const user = userEvent.setup()
    render(
      <MarkdownEditor
        value="<img src=x onerror=alert(1)>"
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    // <img> is not in the allowlist, so the whole element is removed.
    expect(preview.querySelector('img')).toBeNull()
    expect(preview.innerHTML).not.toMatch(/onerror/i)
  })

  it('does not resurrect HTML-encoded <script> entities into live tags', async () => {
    const user = userEvent.setup()
    // NOTE: pass the entities via a JS expression, not a JSX string literal —
    // JSX decodes `&lt;` → `<` at parse time, which would test a raw <script>
    // tag instead of the encoded-entity case we care about here.
    render(
      <MarkdownEditor
        value={'&lt;script&gt;alert(1)&lt;/script&gt;'}
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    // Encoded entities must render as inert text, never as an executable tag:
    // the markup stays escaped (`&lt;script&gt;`) and decodes only as text.
    expect(preview.querySelector('script')).toBeNull()
    expect(preview.innerHTML).not.toMatch(/<script\b/i)
    expect(preview.textContent).toContain('<script>alert(1)</script>')
  })

  it('strips javascript: URLs from anchor hrefs in the preview', async () => {
    const user = userEvent.setup()
    render(
      <MarkdownEditor
        value={'<a href="javascript:alert(1)">x</a>'}
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.innerHTML).not.toMatch(/javascript:/i)
  })

  it('strips on* event handlers from the preview', async () => {
    const user = userEvent.setup()
    render(
      <MarkdownEditor
        value={'<a href="https://example.com" onclick="alert(1)">x</a>'}
        onChange={vi.fn()}
      />
    )

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    const preview = screen.getByTestId('markdown-editor-preview')
    expect(preview.innerHTML).not.toMatch(/onclick=/i)
    // The anchor itself survives with its safe href intact.
    expect(preview.querySelector('a')?.getAttribute('href')).toBe(
      'https://example.com'
    )
  })

  it('shows the empty-preview placeholder when value is blank', async () => {
    const user = userEvent.setup()
    render(<MarkdownEditor value="" onChange={vi.fn()} />)

    await user.click(screen.getByTestId('markdown-editor-preview-tab'))
    expect(screen.getByText(/nothing to preview/i)).toBeInTheDocument()
  })

  it('shows char count when maxLength is provided', () => {
    render(
      <MarkdownEditor value="abc" onChange={vi.fn()} maxLength={100} />
    )
    expect(screen.getByText('3 / 100')).toBeInTheDocument()
  })
})

describe('MarkdownContent', () => {
  it('renders nothing when html is empty', () => {
    const { container } = render(<MarkdownContent html="" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders provided HTML via dangerouslySetInnerHTML', () => {
    render(
      <MarkdownContent
        html="<p><strong>bold</strong> text</p>"
        testId="md-out"
      />
    )
    const el = screen.getByTestId('md-out')
    expect(el.querySelector('strong')?.textContent).toBe('bold')
  })
})
