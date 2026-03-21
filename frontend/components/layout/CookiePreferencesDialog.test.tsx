import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CookiePreferencesDialog } from './CookiePreferencesDialog'

describe('CookiePreferencesDialog', () => {
  const onOpenChange = vi.fn()
  const onSave = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when closed', () => {
    render(
      <CookiePreferencesDialog
        open={false}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.queryByText('Cookie Preferences')).not.toBeInTheDocument()
  })

  it('renders dialog title when open', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText('Cookie Preferences')).toBeInTheDocument()
  })

  it('renders dialog description', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText(/Manage your cookie preferences/)).toBeInTheDocument()
  })

  it('renders Essential Cookies section always checked and disabled', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText('Essential Cookies')).toBeInTheDocument()
    expect(screen.getByText(/Required for authentication and security/)).toBeInTheDocument()

    const essentialSwitch = screen.getByLabelText('Essential cookies (always enabled)')
    expect(essentialSwitch).toBeDisabled()
  })

  it('renders Analytics Cookies section with toggle', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText('Analytics Cookies')).toBeInTheDocument()
    expect(screen.getByText(/Help us understand how you use the site/)).toBeInTheDocument()
    expect(screen.getByLabelText('Analytics cookies')).toBeInTheDocument()
  })

  it('initializes analytics toggle from currentAnalytics=false', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    const analyticsSwitch = screen.getByLabelText('Analytics cookies')
    expect(analyticsSwitch).toHaveAttribute('data-state', 'unchecked')
  })

  it('initializes analytics toggle from currentAnalytics=true', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={true}
        onSave={onSave}
      />
    )
    const analyticsSwitch = screen.getByLabelText('Analytics cookies')
    expect(analyticsSwitch).toHaveAttribute('data-state', 'checked')
  })

  it('calls onSave with analytics=true after toggling on and saving', async () => {
    const user = userEvent.setup()
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )

    // Toggle analytics on
    await user.click(screen.getByLabelText('Analytics cookies'))
    // Click save
    await user.click(screen.getByText('Save Preferences'))

    expect(onSave).toHaveBeenCalledWith(true)
  })

  it('calls onSave with analytics=false when saving with toggle off', async () => {
    const user = userEvent.setup()
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )

    await user.click(screen.getByText('Save Preferences'))
    expect(onSave).toHaveBeenCalledWith(false)
  })

  it('calls onOpenChange(false) when Cancel is clicked', async () => {
    const user = userEvent.setup()
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )

    await user.click(screen.getByText('Cancel'))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows GPC detection notice when gpcSignalDetected is true', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={true}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText(/Global Privacy Control detected/)).toBeInTheDocument()
    expect(screen.getByText(/We respect this signal/)).toBeInTheDocument()
  })

  it('does not show GPC notice when gpcSignalDetected is false', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.queryByText(/Global Privacy Control detected/)).not.toBeInTheDocument()
  })

  it('renders Cancel and Save Preferences buttons', () => {
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )
    expect(screen.getByText('Cancel')).toBeInTheDocument()
    expect(screen.getByText('Save Preferences')).toBeInTheDocument()
  })

  it('can toggle analytics on and then off again before saving', async () => {
    const user = userEvent.setup()
    render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )

    const analyticsSwitch = screen.getByLabelText('Analytics cookies')

    // Toggle on
    await user.click(analyticsSwitch)
    expect(analyticsSwitch).toHaveAttribute('data-state', 'checked')

    // Toggle off
    await user.click(analyticsSwitch)
    expect(analyticsSwitch).toHaveAttribute('data-state', 'unchecked')

    // Save with final state (off)
    await user.click(screen.getByText('Save Preferences'))
    expect(onSave).toHaveBeenCalledWith(false)
  })

  it('resets form state when dialog reopens with different currentAnalytics', () => {
    const { rerender } = render(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={false}
        onSave={onSave}
      />
    )

    expect(screen.getByLabelText('Analytics cookies')).toHaveAttribute('data-state', 'unchecked')

    // Close and reopen with different value
    rerender(
      <CookiePreferencesDialog
        open={false}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={true}
        onSave={onSave}
      />
    )

    rerender(
      <CookiePreferencesDialog
        open={true}
        onOpenChange={onOpenChange}
        gpcSignalDetected={false}
        currentAnalytics={true}
        onSave={onSave}
      />
    )

    // The key prop `${open}-${currentAnalytics}` on CookiePreferencesForm
    // should reset the internal state
    expect(screen.getByLabelText('Analytics cookies')).toHaveAttribute('data-state', 'checked')
  })
})
