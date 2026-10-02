import { describe, it, expect } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { InfoTooltip } from './InfoTooltip'

const COPY = 'Controls immediate alerts when a new show is added.'
const LABEL = 'What do these alerts control?'

function renderInfo(props: Partial<Parameters<typeof InfoTooltip>[0]> = {}) {
  return render(
    <div>
      <p data-testid="outside">Outside the explainer</p>
      <InfoTooltip copy={COPY} label={LABEL} testId="info-glyph" {...props} />
    </div>
  )
}

const trigger = () => screen.getByRole('button', { name: LABEL })

describe('InfoTooltip', () => {
  it('passes the testId through to the trigger button', () => {
    renderInfo()
    expect(screen.getByTestId('info-glyph')).toBe(trigger())
  })

  it('starts collapsed with no explainer rendered', () => {
    renderInfo()
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('opens the popover with the copy on click and sets aria-expanded', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    expect(trigger()).toHaveAttribute('aria-expanded', 'true')
    expect(trigger()).toHaveAttribute('data-state', 'open')
  })

  it('opens the popover on Enter and on Space from keyboard focus', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.tab()
    expect(trigger()).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    trigger().focus()
    await user.keyboard(' ')
    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
  })

  it('opens the popover from a touch tap', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.pointer({ keys: '[TouchA]', target: trigger() })

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    expect(trigger()).toHaveAttribute('aria-expanded', 'true')
  })

  it('closes the popover on an outside pointer down', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')

    await user.click(screen.getByTestId('outside'))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('closes the popover on Escape', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')

    await user.keyboard('{Escape}')

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('toggles the popover closed on a second click of the trigger', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')
    await user.click(trigger())

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('shows the tooltip with the copy on hover without opening the popover', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.hover(trigger())

    expect(await screen.findByRole('tooltip')).toHaveTextContent(COPY)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(trigger()).toHaveAttribute('aria-expanded', 'false')
  })

  it('closes the tooltip when a click opens the popover', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.hover(trigger())
    await screen.findByRole('tooltip')

    await user.click(trigger())

    expect(await screen.findByRole('dialog')).toHaveTextContent(COPY)
    await waitFor(() => expect(screen.queryByRole('tooltip')).toBeNull())
  })

  it('holds the tooltip back while the popover is open', async () => {
    const user = userEvent.setup()
    renderInfo()

    await user.click(trigger())
    await screen.findByRole('dialog')

    await user.unhover(trigger())
    await user.hover(trigger())

    // The tooltip opens after a 120 ms hover delay; wait past it so a
    // regression has time to render before the absence is asserted.
    await new Promise(resolve => setTimeout(resolve, 300))
    expect(screen.queryByRole('tooltip')).toBeNull()
    expect(screen.getByRole('dialog')).toHaveTextContent(COPY)
  })

  it('suppresses long-press text selection and the touch callout on the trigger', () => {
    renderInfo()
    expect(trigger()).toHaveClass('select-none', '[-webkit-touch-callout:none]')
  })
})
