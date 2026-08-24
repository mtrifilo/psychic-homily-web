import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AlertChipRadioGroup } from './AlertChipRadioGroup'

// The chip radiogroup shared by the artist/venue scope reveal and the scene
// notify-mode toggle (PSY-1905). Its whole justification is the ARIA contract,
// so the keyboard behaviour is the part that most needs pinning.

const OPTIONS = [
  { value: 'near_me', label: 'Near me' },
  { value: 'everywhere', label: 'Everywhere' },
  { value: 'off', label: 'Off' },
] as const

const onChange = vi.fn()

const renderGroup = (
  props: Partial<Parameters<typeof AlertChipRadioGroup>[0]> = {}
) =>
  renderWithProviders(
    <AlertChipRadioGroup
      ariaLabel="Alerts for Alpha"
      label="Alerts:"
      options={OPTIONS}
      value="near_me"
      onChange={onChange}
      {...props}
    />
  )

describe('AlertChipRadioGroup', () => {
  beforeEach(() => {
    onChange.mockReset()
  })

  it('marks the selected option and names the group', () => {
    renderGroup()
    expect(
      screen.getByRole('radiogroup', { name: 'Alerts for Alpha' })
    ).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Near me' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Off' })).not.toBeChecked()
  })

  // A radiogroup is ONE tab stop. Leaving every chip tabbable announces
  // "radio 1 of 3" and then navigates like three separate buttons.
  it('exposes exactly one tab stop, on the selected option', () => {
    renderGroup({ value: 'everywhere' })
    expect(screen.getByRole('radio', { name: 'Near me' })).toHaveAttribute(
      'tabindex',
      '-1'
    )
    expect(screen.getByRole('radio', { name: 'Everywhere' })).toHaveAttribute(
      'tabindex',
      '0'
    )
    expect(screen.getByRole('radio', { name: 'Off' })).toHaveAttribute(
      'tabindex',
      '-1'
    )
  })

  it('moves focus with the arrow keys, wrapping in both directions', async () => {
    const user = userEvent.setup()
    renderGroup()

    await user.tab()
    expect(screen.getByRole('radio', { name: 'Near me' })).toHaveFocus()

    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('radio', { name: 'Everywhere' })).toHaveFocus()

    await user.keyboard('{ArrowRight}{ArrowRight}')
    expect(screen.getByRole('radio', { name: 'Near me' })).toHaveFocus()

    await user.keyboard('{ArrowLeft}')
    expect(screen.getByRole('radio', { name: 'Off' })).toHaveFocus()
  })

  it('jumps to the ends with Home and End', async () => {
    const user = userEvent.setup()
    renderGroup({ value: 'everywhere' })

    await user.tab()
    await user.keyboard('{End}')
    expect(screen.getByRole('radio', { name: 'Off' })).toHaveFocus()

    await user.keyboard('{Home}')
    expect(screen.getByRole('radio', { name: 'Near me' })).toHaveFocus()
  })

  // Selection must NOT follow focus here. Arrowing past a chip on the way to
  // the one you want would STORE it: passing through "Everywhere" on an artist
  // overwrites the near-me scope the read path goes out of its way to
  // preserve, and it spends one PATCH per keystroke.
  it('does not write while arrowing across the options', async () => {
    const user = userEvent.setup()
    renderGroup()

    await user.tab()
    await user.keyboard('{ArrowRight}{ArrowRight}{ArrowLeft}')

    expect(onChange).not.toHaveBeenCalled()
  })

  it('commits the focused option on Space or Enter', async () => {
    const user = userEvent.setup()
    renderGroup()

    await user.tab()
    await user.keyboard('{ArrowRight}')
    await user.keyboard(' ')
    expect(onChange).toHaveBeenCalledWith('everywhere')

    onChange.mockReset()
    await user.keyboard('{ArrowRight}')
    await user.keyboard('{Enter}')
    expect(onChange).toHaveBeenCalledWith('off')
  })

  it('commits on click', async () => {
    const user = userEvent.setup()
    renderGroup()

    await user.click(screen.getByRole('radio', { name: 'Off' }))
    expect(onChange).toHaveBeenCalledWith('off')
  })

  it('does not re-write the option already selected', async () => {
    const user = userEvent.setup()
    renderGroup()

    await user.click(screen.getByRole('radio', { name: 'Near me' }))
    expect(onChange).not.toHaveBeenCalled()
  })

  // A DISABLED button cannot hold focus, so parking the group with `disabled`
  // during a write ejects the keyboard user to <body> and the next Tab
  // restarts from the top of the document.
  it('stays focusable while a write is in flight', async () => {
    const user = userEvent.setup()
    renderGroup({ pending: true })

    const chip = screen.getByRole('radio', { name: 'Near me' })
    expect(chip).not.toBeDisabled()
    expect(chip).toHaveAttribute('aria-disabled', 'true')

    await user.tab()
    expect(chip).toHaveFocus()
  })

  it('blocks writes while a write is in flight', async () => {
    const user = userEvent.setup()
    renderGroup({ pending: true })

    await user.click(screen.getByRole('radio', { name: 'Off' }))
    expect(onChange).not.toHaveBeenCalled()
  })
})
