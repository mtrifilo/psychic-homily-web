import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ComponentType } from 'react'

/**
 * `MDXRemote` from next-mdx-remote/rsc is an async server component that
 * compiles MDX in the RSC runtime — it cannot execute under jsdom/vitest.
 * We mock it to a passthrough so we can assert what MDXContent itself
 * controls: the prose wrapper, the forwarded `source`, and the custom
 * `components` map (embeds + styled element overrides). The map's render
 * functions are then exercised directly since they are plain components.
 */
type MDXRemoteProps = {
  source: string
  components: Record<string, ComponentType<Record<string, unknown>>>
}

const mdxRemoteSpy = vi.fn()

vi.mock('next-mdx-remote/rsc', () => ({
  MDXRemote: (props: MDXRemoteProps) => {
    mdxRemoteSpy(props)
    return <div data-testid="mdx-rendered">{props.source}</div>
  },
}))

import { MDXContent } from './mdx-content'

function getComponentsMap(): MDXRemoteProps['components'] {
  render(<MDXContent source="content" />)
  return mdxRemoteSpy.mock.calls.at(-1)![0].components
}

describe('MDXContent', () => {
  it('wraps rendered MDX in a prose container', () => {
    const { container } = render(<MDXContent source="# Hello" />)
    const wrapper = container.querySelector('div.prose')
    expect(wrapper).toBeInTheDocument()
    expect(wrapper?.className).toContain('max-w-none')
  })

  it('forwards the source string to MDXRemote', () => {
    const source = '# Heading\n\nbody'
    render(<MDXContent source={source} />)
    expect(screen.getByTestId('mdx-rendered')).toHaveTextContent('# Heading')
    expect(mdxRemoteSpy.mock.calls.at(-1)![0].source).toBe(source)
  })

  it('registers the Bandcamp and SoundCloud embeds as MDX components', () => {
    const components = getComponentsMap()
    expect(components.Bandcamp).toBeDefined()
    expect(components.SoundCloud).toBeDefined()
  })

  it('styles code with a monospace highlight class', () => {
    const Code = getComponentsMap().code
    const { container } = render(<Code>const x = 1</Code>)
    const code = container.querySelector('code')
    expect(code).toHaveTextContent('const x = 1')
    expect(code?.className).toContain('font-mono')
    expect(code?.className).toContain('bg-muted')
  })

  it('styles pre code blocks with a highlight background', () => {
    const Pre = getComponentsMap().pre
    const { container } = render(<Pre>block</Pre>)
    const pre = container.querySelector('pre')
    expect(pre?.className).toContain('bg-muted')
    expect(pre?.className).toContain('overflow-x-auto')
  })

  it('opens external anchors in a new tab with safe rel', () => {
    const Anchor = getComponentsMap().a
    const { container } = render(
      <Anchor href="https://example.com">external</Anchor>
    )
    const anchor = container.querySelector('a')
    expect(anchor).toHaveAttribute('target', '_blank')
    expect(anchor).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('keeps internal anchors in the same tab', () => {
    const Anchor = getComponentsMap().a
    const { container } = render(<Anchor href="/blog">internal</Anchor>)
    const anchor = container.querySelector('a')
    expect(anchor).not.toHaveAttribute('target')
    expect(anchor).not.toHaveAttribute('rel')
  })
})
