import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AttendanceButton } from './AttendanceButton'
import type { AttendanceCounts } from '../types'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  isAuthenticated: false,
  user: null,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/shows/test-show',
}))

// Mock attendance hooks
const mockSetAttendanceMutate = vi.fn()
const mockRemoveAttendanceMutate = vi.fn()
const mockUseShowAttendance = vi.fn((_showId?: number) => ({ data: undefined as unknown }))

vi.mock('../hooks/useAttendance', () => ({
  useShowAttendance: (showId: number) => mockUseShowAttendance(showId),
  useSetAttendance: () => ({
    mutate: mockSetAttendanceMutate,
    isPending: false,
  }),
  useRemoveAttendance: () => ({
    mutate: mockRemoveAttendanceMutate,
    isPending: false,
  }),
}))

// Mock tooltip (render content directly, no portal)
vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <span data-testid="tooltip">{children}</span>,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, disabled, onClick, ...props }: {
    children: React.ReactNode
    disabled?: boolean
    onClick?: (e: React.MouseEvent) => void
    [key: string]: unknown
  }) => (
    <button
      disabled={disabled}
      onClick={onClick}
      aria-label={props['aria-label'] as string}
    >
      {children}
    </button>
  ),
}))

describe('AttendanceButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseShowAttendance.mockReturnValue({ data: undefined })
  })

  describe('compact mode', () => {
    it('renders going and interested buttons', () => {
      render(<AttendanceButton showId={1} compact />)
      expect(screen.getByLabelText('Going')).toBeInTheDocument()
      expect(screen.getByLabelText('Interested')).toBeInTheDocument()
    })

    it('shows going count when > 0', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 5,
        interested_count: 0,
        user_status: '',
      }
      render(<AttendanceButton showId={1} compact attendanceData={attendanceData} />)
      expect(screen.getByText('5')).toBeInTheDocument()
      expect(screen.getByLabelText('Going 5')).toBeInTheDocument()
    })

    it('shows interested count when > 0', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 0,
        interested_count: 3,
        user_status: '',
      }
      render(<AttendanceButton showId={1} compact attendanceData={attendanceData} />)
      expect(screen.getByText('3')).toBeInTheDocument()
      expect(screen.getByLabelText('Interested 3')).toBeInTheDocument()
    })

    it('does not show counts when 0', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 0,
        interested_count: 0,
        user_status: '',
      }
      render(<AttendanceButton showId={1} compact attendanceData={attendanceData} />)
      expect(screen.queryByText('0')).not.toBeInTheDocument()
    })

    it('calls setAttendance with "going" on click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={42} compact />)
      await user.click(screen.getByLabelText('Going'))
      expect(mockSetAttendanceMutate).toHaveBeenCalledWith({ showId: 42, status: 'going' })
    })

    it('calls setAttendance with "interested" on click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={42} compact />)
      await user.click(screen.getByLabelText('Interested'))
      expect(mockSetAttendanceMutate).toHaveBeenCalledWith({ showId: 42, status: 'interested' })
    })

    it('calls removeAttendance when toggling off current status', async () => {
      const user = userEvent.setup()
      const attendanceData: AttendanceCounts = {
        show_id: 42,
        going_count: 1,
        interested_count: 0,
        user_status: 'going',
      }
      render(<AttendanceButton showId={42} compact attendanceData={attendanceData} />)
      await user.click(screen.getByLabelText('Going 1'))
      expect(mockRemoveAttendanceMutate).toHaveBeenCalledWith(42)
    })

    it('switches status when clicking different button', async () => {
      const user = userEvent.setup()
      const attendanceData: AttendanceCounts = {
        show_id: 42,
        going_count: 1,
        interested_count: 0,
        user_status: 'going',
      }
      render(<AttendanceButton showId={42} compact attendanceData={attendanceData} />)
      await user.click(screen.getByLabelText('Interested'))
      expect(mockSetAttendanceMutate).toHaveBeenCalledWith({ showId: 42, status: 'interested' })
    })

    it('shows tooltip text for going button', () => {
      render(<AttendanceButton showId={1} compact />)
      expect(screen.getByText('Mark as going')).toBeInTheDocument()
    })

    it('shows tooltip text for interested button', () => {
      render(<AttendanceButton showId={1} compact />)
      expect(screen.getByText('Mark as interested')).toBeInTheDocument()
    })

    it('shows "Remove going status" tooltip when already going', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 1,
        interested_count: 0,
        user_status: 'going',
      }
      render(<AttendanceButton showId={1} compact attendanceData={attendanceData} />)
      expect(screen.getByText('Remove going status')).toBeInTheDocument()
    })

    it('shows "Remove interested status" tooltip when already interested', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 0,
        interested_count: 1,
        user_status: 'interested',
      }
      render(<AttendanceButton showId={1} compact attendanceData={attendanceData} />)
      expect(screen.getByText('Remove interested status')).toBeInTheDocument()
    })
  })

  describe('full mode', () => {
    it('renders going and interested buttons with labels', () => {
      render(<AttendanceButton showId={1} />)
      expect(screen.getByText('Going')).toBeInTheDocument()
      expect(screen.getByText('Interested')).toBeInTheDocument()
    })

    it('shows going count badge when > 0', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 12,
        interested_count: 0,
        user_status: '',
      }
      render(<AttendanceButton showId={1} attendanceData={attendanceData} />)
      expect(screen.getByText('12')).toBeInTheDocument()
    })

    it('shows interested count badge when > 0', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 0,
        interested_count: 7,
        user_status: '',
      }
      render(<AttendanceButton showId={1} attendanceData={attendanceData} />)
      expect(screen.getByText('7')).toBeInTheDocument()
    })

    it('calls setAttendance on going click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={42} />)
      await user.click(screen.getByText('Going'))
      expect(mockSetAttendanceMutate).toHaveBeenCalledWith({ showId: 42, status: 'going' })
    })

    it('calls setAttendance on interested click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={42} />)
      await user.click(screen.getByText('Interested'))
      expect(mockSetAttendanceMutate).toHaveBeenCalledWith({ showId: 42, status: 'interested' })
    })
  })

  describe('unauthenticated user', () => {
    beforeEach(() => {
      mockAuthContext.mockReturnValue({
        isAuthenticated: false,
        user: null,
        isLoading: false,
        logout: vi.fn(),
      })
    })

    it('redirects to auth page on compact going click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={1} compact />)
      await user.click(screen.getByLabelText('Going'))
      expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fshows%2Ftest-show')
      expect(mockSetAttendanceMutate).not.toHaveBeenCalled()
    })

    it('redirects to auth page on full going click', async () => {
      const user = userEvent.setup()
      render(<AttendanceButton showId={1} />)
      await user.click(screen.getByText('Going'))
      expect(mockPush).toHaveBeenCalledWith('/auth?returnTo=%2Fshows%2Ftest-show')
      expect(mockSetAttendanceMutate).not.toHaveBeenCalled()
    })

    it('shows "Sign in to RSVP" tooltip in compact mode', () => {
      render(<AttendanceButton showId={1} compact />)
      const tooltips = screen.getAllByText('Sign in to RSVP')
      expect(tooltips.length).toBe(2) // one for each button
    })
  })

  describe('batch attendance data', () => {
    it('uses provided attendanceData instead of fetching', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 10,
        interested_count: 5,
        user_status: 'going',
      }
      render(<AttendanceButton showId={1} attendanceData={attendanceData} />)
      // Should show counts from batch data
      expect(screen.getByText('10')).toBeInTheDocument()
      expect(screen.getByText('5')).toBeInTheDocument()
    })

    it('disables fetch when attendanceData is provided', () => {
      const attendanceData: AttendanceCounts = {
        show_id: 1,
        going_count: 0,
        interested_count: 0,
        user_status: '',
      }
      render(<AttendanceButton showId={1} attendanceData={attendanceData} />)
      // useShowAttendance should be called with 0 (disabled) when batch data exists
      expect(mockUseShowAttendance).toHaveBeenCalledWith(0)
    })

    it('fetches individually when no attendanceData provided', () => {
      render(<AttendanceButton showId={42} />)
      expect(mockUseShowAttendance).toHaveBeenCalledWith(42)
    })
  })
})
