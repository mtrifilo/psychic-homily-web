import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

import { MarkdownContent } from './MarkdownContent'

// MarkdownContent split out of MarkdownEditor in PSY-951 — these cases moved
// here verbatim so the read-only renderer keeps its own coverage. It renders
// server-sanitized HTML only (no marked/dompurify), so the security contract
// is "render exactly what the backend sanitized" — see the module's doc note.
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
